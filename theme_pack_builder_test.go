package main

import (
	"os"
	"errors"
	"syscall"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCreatePackedThemeFileArchivesStandaloneSources(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil { t.Fatal(err) }
	first, err := createThemePresetFile(ThemePreset{Name:"Forest", Colors:defaultTheme()})
	if err != nil { t.Fatal(err) }
	second, err := createThemePresetFile(ThemePreset{Name:"Desert", Colors:defaultTheme()})
	if err != nil { t.Fatal(err) }
	filename, id, err := createPackedThemeFile("Field Collection", []themeListEntry{first, second})
	if err != nil { t.Fatal(err) }
	if filename != "pack-field-collection.toml" || id != first.Preset.ID {
		t.Fatalf("created %q with first ID %q", filename, id)
	}
	packs, err := loadThemePacks()
	if err != nil { t.Fatal(err) }
	var packed ThemePack
	counts := map[string]int{}
	for _, pack := range packs {
		for _, preset := range pack.Themes { counts[preset.Name]++ }
		if pack.FileName == filename { packed = pack }
	}
	if len(packed.Themes) != 2 || packed.PackName != "Field Collection" {
		t.Fatalf("unexpected saved pack: %+v", packed)
	}
	if counts["Forest"] != 1 || counts["Desert"] != 1 {
		t.Fatalf("packed themes duplicated in browser: %+v", counts)
	}
	for _, source := range []themeListEntry{first,second} {
		if _, err := os.Stat(filepath.Join(themesDir(), source.FileName)); !os.IsNotExist(err) {
			t.Fatalf("standalone source remains live: %q err=%v", source.FileName, err)
		}
		if _, err := os.Stat(filepath.Join(themesDir(), "packed", source.FileName)); err != nil {
			t.Fatalf("source %q was not preserved in packed archive: %v", source.FileName, err)
		}
	}
	if _, _, err := createPackedThemeFile("Field Collection", []themeListEntry{first}); err == nil {
		t.Fatal("duplicate pack filename should not overwrite an existing pack")
	}
}

func TestPackingThemesFromMultiThemePackKeepsUnselectedAndEditable(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil { t.Fatal(err) }
	presets := []ThemePreset{
		{ID:"a",Name:"Alpha",Colors:defaultTheme()},
		{ID:"b",Name:"Bravo",Colors:defaultTheme()},
		{ID:"c",Name:"Charlie",Colors:defaultTheme()},
	}
	source := ThemePack{SchemaVersion:themePackSchemaVersion,PackName:"Original Pack",Themes:presets}
	if err := saveThemePack("original.toml",source); err != nil { t.Fatal(err) }
	selected := []themeListEntry{
		{Preset:presets[0],FileName:"original.toml",ThemeCount:3,PackName:"Original Pack"},
		{Preset:presets[1],FileName:"original.toml",ThemeCount:3,PackName:"Original Pack"},
	}
	filename, _, err := createPackedThemeFile("New Pack",selected)
	if err != nil { t.Fatal(err) }
	data, err := os.ReadFile(filepath.Join(themesDir(),"original.toml"))
	if err != nil { t.Fatal(err) }
	remaining, err := decodeThemePackBytes(data)
	if err != nil { t.Fatal(err) }
	if len(remaining.Themes)!=1 || remaining.Themes[0].ID!="c" {
		t.Fatalf("unselected source themes changed: %+v",remaining.Themes)
	}
	data, err = os.ReadFile(filepath.Join(themesDir(),"packed","original.toml"))
	if err != nil { t.Fatal(err) }
	backup, err := decodeThemePackBytes(data)
	if err != nil { t.Fatal(err) }
	if len(backup.Themes)!=3 {t.Fatalf("original pack was not archived intact: %+v",backup.Themes)}
	data, err = os.ReadFile(filepath.Join(themesDir(),filename))
	if err != nil {t.Fatal(err)}
	newPack, err := decodeThemePackBytes(data)
	if err != nil {t.Fatal(err)}
	if len(newPack.Themes)!=2 {t.Fatalf("new pack contains %+v",newPack.Themes)}
	editable := themeListEntry{Preset:newPack.Themes[0],FileName:filename,PackName:newPack.PackName,ThemeCount:2}
	if !themeEntryEditable(editable) {t.Fatal("non-built-in pack theme should be editable")}
	changed := editable.Preset
	changed.Description = "Edited inside new pack"
	if _, err := saveThemePresetFile(editable,changed); err != nil {t.Fatal(err)}
	reloaded, err := loadThemePacks()
	if err != nil {t.Fatal(err)}
	for _, pack := range reloaded {
		if pack.FileName==filename && pack.Themes[0].Description!="Edited inside new pack" {
			t.Fatal("could not save theme edits inside a multi-theme pack")
		}
	}
}

func TestThemePackBuilderButtonSelectionAndNaming(t *testing.T) {
	m := newThemeEditorTestModel(t)
	// Built-in entries must not expose the pack button.
	_, rightWidth := m.themeBrowserColumnWidths()
	rightStart := m.themeBrowserDividerX()+1
	packX := rightStart+rightWidth-len(themePackButtonLabel)
	if m.themePackButtonContains(packX,2) ||
		strings.Contains(stripANSISequences(m.themeDetailsRows(rightWidth)[0]),themePackButtonLabel) {
		t.Fatal("built-in pack unexpectedly offers packing")
	}
	m.cloneSelectedTheme()
	custom, ok := m.selectedThemeEntry()
	if !ok {t.Fatal("clone did not select a custom theme")}
	_, rightWidth = m.themeBrowserColumnWidths()
	rightStart = m.themeBrowserDividerX()+1
	packX = rightStart+rightWidth-len(themePackButtonLabel)
	if !m.themePackButtonContains(packX,2) {
		t.Fatal("custom theme does not have clickable right-pane [PACK]")
	}
	updated, _ := m.Update(tea.MouseClickMsg{X:packX,Y:2,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.themePacking || len(m.themePackSelected)!=1 || !m.themePackSelected[themePackSelectionKey(custom)] {
		t.Fatal("[PACK] did not start selecting the current custom theme")
	}
	if m.themeCreateButtonContains(m.themeBrowserContentStartX()+10,2) {
		t.Fatal("create action active during pack selection")
	}
	if m.themeApplyButtonContains(rightStart,2+m.themeActionButtonRowForWidth(rightWidth)) {
		t.Fatal("normal Apply button remains clickable in pack mode")
	}
	lines := stripANSISequences(m.renderThemeBrowser())
	if !strings.Contains(lines,"# "+custom.Preset.Name) {
		t.Fatalf("selected theme lacks # marker: %q",lines)
	}
	// Built-in is muted and Space must not mark it.
	m.selectThemeEntry(0)
	m.toggleThemePacking(0)
	if len(m.themePackSelected)!=1 {
		t.Fatal("default theme was selected for packing")
	}
	for i, entry := range m.themeEntries {
		if entry.FileName == custom.FileName && entry.Preset.ID == custom.Preset.ID {
			m.selectThemeEntry(i)
			break
		}
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code:' ',Text:" "})
	m = updated.(model)
	if len(m.themePackSelected)!=0 {
		t.Fatal("spacebar did not remove # selection")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code:' ',Text:" "})
	m = updated.(model)
	if len(m.themePackSelected)!=1 {
		t.Fatal("spacebar did not restore # selection")
	}
	// Confirm via right-pane button and save through the composer.
	confirmX := rightStart+rightWidth-len(themePackConfirmLabel)
	if !m.themePackButtonContains(confirmX,2) {t.Fatal("[CONFIRM] hitbox missing")}
	updated, _ = m.Update(tea.MouseClickMsg{X:confirmX,Y:2,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.themePackNaming {t.Fatal("confirm did not open pack naming")}
	m.input.SetValue("My Collection")
	updated, _ = m.Update(tea.KeyPressMsg{Code:tea.KeyEnter})
	m = updated.(model)
	if m.themePacking || m.themePackNaming || m.themeEditError!="" {
		t.Fatalf("pack not saved: packing=%v naming=%v error=%q",m.themePacking,m.themePackNaming,m.themeEditError)
	}
	entry, ok := m.selectedThemeEntry()
	if !ok || entry.FileName!="pack-my-collection.toml" || !themeEntryEditable(entry) {
		t.Fatalf("new pack not selected and editable: %+v",entry)
	}
}

func TestPackModeFooterRetainsClickTargetsDuringResize(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.cloneSelectedTheme()
	m.beginThemePacking()
	for _, naming := range []bool{false,true} {
		if naming {m.beginThemePackNaming()}
		for _, width := range []int{100,72,44,28,20,16,12} {
			m.width = width
			line, targets := m.renderStatusBar(false,false)
			plain := stripANSISequences(line)
			cells := []rune(plain)
			if len(cells)!=width {t.Fatalf("width %d: footer has %d cells",width,len(cells))}
			if targets.hasApply || strings.Contains(plain,"Apply") {
				t.Fatalf("pack mode footer offers Apply: %q",plain)
			}
			if naming && targets.hasPageScroll {
				t.Fatalf("pack naming incorrectly exposes PgUp/PgDn click targets: %+v %q",targets,plain)
			}
			if targets.hasSend {
				t.Fatalf("pack mode should not expose chat Send: %+v %q",targets,plain)
			}
			if targets.quitEnd>len(cells) || string(cells[targets.quitStart:targets.quitEnd])!="Quit" {
				t.Fatalf("width %d: Quit hitbox mismatch: %+v %q",width,targets,plain)
			}
			if targets.hasSend && (targets.sendEnd!=width-1 || targets.quitEnd>targets.sendStart) {
				t.Fatalf("width %d: Send overlaps Quit: %+v %q",width,targets,plain)
			}
		}
	}
}

func TestPackBuilderRenamesDuplicateIDsWithoutChangingSources(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {t.Fatal(err)}
	first := ThemePack{SchemaVersion:themePackSchemaVersion,PackName:"One",Themes:[]ThemePreset{{ID:"shared",Name:"First",Colors:defaultTheme()}}}
	second := ThemePack{SchemaVersion:themePackSchemaVersion,PackName:"Two",Themes:[]ThemePreset{{ID:"shared",Name:"Second",Colors:defaultTheme()}}}
	if err := saveThemePack("one.toml",first); err != nil {t.Fatal(err)}
	if err := saveThemePack("two.toml",second); err != nil {t.Fatal(err)}
	selected := []themeListEntry{
		{Preset:first.Themes[0],FileName:"one.toml"},
		{Preset:second.Themes[0],FileName:"two.toml"},
	}
	filename, _, err := createPackedThemeFile("Combined",selected)
	if err != nil {t.Fatal(err)}
	data, err := os.ReadFile(filepath.Join(themesDir(),filename))
	if err != nil {t.Fatal(err)}
	pack, err := decodeThemePackBytes(data)
	if err != nil {t.Fatal(err)}
	if pack.Themes[0].ID!="shared" || pack.Themes[1].ID!="shared-2" {
		t.Fatalf("duplicate IDs not reconciled: %+v",pack.Themes)
	}
	for _, filename := range []string{"one.toml","two.toml"} {
		if _, err := os.Stat(filepath.Join(themesDir(),"packed",filename)); err != nil {
			t.Fatalf("missing source backup %q: %v",filename,err)
		}
	}
}

func TestPackBuilderCannotIncludeBuiltinsOrOverwritePack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {t.Fatal(err)}
	builtin := themeListEntry{Preset:defaultThemePack().Themes[0],FileName:defaultThemePackFilename}
	if _, _, err := createPackedThemeFile("Illegal",[]themeListEntry{builtin}); err == nil {
		t.Fatal("default pack theme was allowed into custom pack")
	}
	if _, err := os.Stat(filepath.Join(themesDir(),"pack-illegal.toml")); !os.IsNotExist(err) {
		t.Fatal("rejected built-in source nevertheless created a pack")
	}
}

func TestPackBuilderCancelRestoresComposerAndSelection(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.cloneSelectedTheme()
	m.input.SetValue("unsent chat message")
	m.beginThemePacking()
	if !m.themePacking {t.Fatal("could not enter pack mode")}
	m.beginThemePackNaming()
	if !m.themePackNaming {t.Fatal("could not enter naming mode")}
	m.input.SetValue("temporary pack name")
	m.cancelThemePackNaming()
	if m.themePackNaming || m.input.Value()!="unsent chat message" {
		t.Fatalf("cancel did not restore composer: naming=%v value=%q",m.themePackNaming,m.input.Value())
	}
	m.cancelThemePacking()
	if m.themePacking || len(m.themePackSelected)!=0 {
		t.Fatal("cancel did not discard the temporary selections")
	}
}

func TestSingleThemePackKeepsNameAndFilenameWhenPresetIsRenamed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME",t.TempDir())
	if _, err := loadConfig(); err != nil {t.Fatal(err)}
	entry, err := createThemePresetFile(ThemePreset{Name:"Original",Colors:defaultTheme()})
	if err != nil {t.Fatal(err)}
	filename, id, err := createPackedThemeFile("Keep This Pack",[]themeListEntry{entry})
	if err != nil {t.Fatal(err)}
	data, err := os.ReadFile(filepath.Join(themesDir(),filename))
	if err != nil {t.Fatal(err)}
	pack, err := decodeThemePackBytes(data)
	if err != nil {t.Fatal(err)}
	bundled := themeListEntry{Preset:pack.Themes[0],FileName:filename,PackName:pack.PackName,ThemeCount:1}
	if id!=bundled.Preset.ID || !themeEntryEditable(bundled) {
		t.Fatal("single-theme pack did not preserve editable ID")
	}
	renamed := bundled.Preset
	renamed.Name = "Renamed"
	updated, err := saveThemePresetFile(bundled,renamed)
	if err != nil {t.Fatal(err)}
	if updated.FileName!=filename {
		t.Fatalf("renaming a packed theme renamed its pack file from %q to %q",filename,updated.FileName)
	}
	data, err = os.ReadFile(filepath.Join(themesDir(),filename))
	if err != nil {t.Fatal(err)}
	pack, err = decodeThemePackBytes(data)
	if err != nil {t.Fatal(err)}
	if pack.PackName!="Keep This Pack" || pack.Themes[0].Name!="Renamed" {
		t.Fatalf("pack identity not preserved: %+v",pack)
	}
}

func TestPackButtonRemainsReachableWithLongThemeName(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.cloneSelectedTheme()
	entry, ok := m.selectedThemeEntry()
	if !ok {t.Fatal("no custom theme")}
	entry.Preset.Name = strings.Repeat("VeryLongName",12)
	m.themeEntries[m.themeSelected] = entry
	_, width := m.themeBrowserColumnWidths()
	row := stripANSISequences(m.themeDetailsRows(width)[0])
	if !strings.HasSuffix(row,themePackButtonLabel) {
		t.Fatalf("long title hid pack button: %q",row)
	}
	rightStart := m.themeBrowserDividerX()+1
	if !m.themePackButtonContains(rightStart+width-len(themePackButtonLabel),2) {
		t.Fatal("long title hides pack click target")
	}
}

func TestPackCreationFailuresNeverLoseOriginalFiles(t *testing.T) {
	for _, tc := range []struct{
		name string
		writeFailure int
		renameFailure int
		failure error
	}{
		{name:"disk full writing pack",writeFailure:1,failure:syscall.ENOSPC},
		{name:"permissions writing remainder",writeFailure:2,failure:os.ErrPermission},
		{name:"cannot archive first source",renameFailure:1,failure:os.ErrPermission},
		{name:"cannot install remaining themes",renameFailure:2,failure:syscall.ENOSPC},
		{name:"cannot archive second source",renameFailure:3,failure:os.ErrPermission},
		{name:"cannot publish pack",renameFailure:4,failure:syscall.ENOSPC},
	} {
		t.Run(tc.name,func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME",t.TempDir())
			if _, err := loadConfig(); err != nil {t.Fatal(err)}
			presets := []ThemePreset{
				{ID:"a",Name:"Alpha",Colors:defaultTheme()},
				{ID:"b",Name:"Bravo",Colors:defaultTheme()},
			}
			srcPack := ThemePack{SchemaVersion:themePackSchemaVersion,PackName:"Original",Themes:presets}
			if err := saveThemePack("original.toml",srcPack); err != nil {t.Fatal(err)}
			other, err := createThemePresetFile(ThemePreset{Name:"Charlie",Colors:defaultTheme()})
			if err != nil {t.Fatal(err)}
			originalPath := filepath.Join(themesDir(),"original.toml")
			otherPath := filepath.Join(themesDir(),other.FileName)
			originalBytes, err := os.ReadFile(originalPath)
			if err != nil {t.Fatal(err)}
			otherBytes, err := os.ReadFile(otherPath)
			if err != nil {t.Fatal(err)}
			selected := []themeListEntry{
				{Preset:presets[0],FileName:"original.toml"},
				other,
			}
			writes, renames := 0,0
			ops := defaultPackFileOps()
			ops.write = func(path string,data []byte) error {
				writes++
				if writes==tc.writeFailure {return tc.failure}
				return os.WriteFile(path,data,0600)
			}
			ops.rename = func(from,to string) error {
				renames++
				if renames==tc.renameFailure {return tc.failure}
				return os.Rename(from,to)
			}
			_, _, err = createPackedThemeFileWithOps("Should Fail",selected,ops)
			if err==nil || !errors.Is(err,tc.failure) {
				t.Fatalf("expected injected failure %v, got %v",tc.failure,err)
			}
			for path,want := range map[string][]byte{originalPath:originalBytes,otherPath:otherBytes} {
				got, err := os.ReadFile(path)
				if err != nil || string(got)!=string(want) {
					t.Fatalf("original %q not restored: err=%v bytes=%q",path,err,got)
				}
			}
			if _, err := os.Stat(filepath.Join(themesDir(),"pack-should-fail.toml")); !os.IsNotExist(err) {
				t.Fatalf("partially published pack is visible: %v",err)
			}
			if entries, err := os.ReadDir(themesDir()); err != nil {
				t.Fatal(err)
			} else {
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(),".pack-staging-") {
						t.Fatalf("left behind staging directory: %q",entry.Name())
					}
				}
			}
			packs, err := loadThemePacks()
			if err != nil {t.Fatalf("library broken after failed transaction: %v",err)}
			counts := map[string]int{}
			for _, pack := range packs {
				for _, preset := range pack.Themes {counts[preset.Name]++}
			}
			if counts["Alpha"]!=1 || counts["Bravo"]!=1 || counts["Charlie"]!=1 {
				t.Fatalf("source themes changed after failed packing: %+v",counts)
			}
		})
	}
}

func TestPackCreationReportsIncompleteRollbackAndKeepsArchive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME",t.TempDir())
	if _, err := loadConfig(); err != nil {t.Fatal(err)}
	entry, err := createThemePresetFile(ThemePreset{Name:"Recovery Test",Colors:defaultTheme()})
	if err != nil {t.Fatal(err)}
	count := 0
	ops := defaultPackFileOps()
	ops.rename = func(from,to string) error {
		count++
		if count == 2 {return syscall.ENOSPC} // Fail final publish after archiving.
		return os.Rename(from,to)
	}
	ops.restore = func(from,to string) error {
		return os.ErrPermission // Simulate unsuccessful rollback.
	}
	_, _, err = createPackedThemeFileWithOps("Recovery Test Pack",[]themeListEntry{entry},ops)
	if !errors.Is(err,syscall.ENOSPC) || !errors.Is(err,os.ErrPermission) {
		t.Fatalf("expected both publishing and rollback errors, got %v",err)
	}
	if !strings.Contains(err.Error(),"automatic recovery was incomplete") ||
		!strings.Contains(err.Error(),"themes/packed/") {
		t.Fatalf("recovery instructions were omitted: %v",err)
	}
	archived := filepath.Join(themesDir(),"packed",entry.FileName)
	if _, err := os.Stat(archived); err != nil {
		t.Fatalf("original source was not preserved for manual recovery: %v",err)
	}
	if _, err := os.Stat(filepath.Join(themesDir(),"pack-recovery-test-pack.toml")); !os.IsNotExist(err) {
		t.Fatalf("failed pack should not appear in library: %v",err)
	}
	// The injected restore failure must not permanently damage the test
	// environment: confirm that the archived original can be restored.
	if err := os.Rename(archived,filepath.Join(themesDir(),entry.FileName)); err != nil {
		t.Fatalf("archived source is not recoverable: %v",err)
	}
}

func TestPackFailureAppearsInThemePackingPane(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.cloneSelectedTheme()
	m.beginThemePacking()
	m.beginThemePackNaming()
	m.input.SetValue("Bad Pack")
	m.themeEditError = packCreationError(syscall.ENOSPC)
	_, width := m.themeBrowserColumnWidths()
	plain := stripANSISequences(strings.Join(m.themeDetailsRows(width),"\n"))
	if !strings.Contains(plain,"PACKING ERROR") ||
		!strings.Contains(plain,"Couldn't complete packing (disk full)") {
		t.Fatalf("pack failure not shown in details pane: %q",plain)
	}
	if !m.themePackNaming || m.input.Value()!="Bad Pack" {
		t.Fatal("showing an error unexpectedly closed pack naming or erased the name")
	}
	if got := packCreationError(os.ErrPermission); !strings.Contains(got,"permissions") {
		t.Fatalf("missing actionable permission error: %q",got)
	}
}
