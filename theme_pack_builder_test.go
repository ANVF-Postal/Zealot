package main

import (
	"os"
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
			if targets.quitEnd>len(cells) || string(cells[targets.quitStart:targets.quitEnd])!="Quit" {
				t.Fatalf("width %d: Quit hitbox mismatch: %+v %q",width,targets,plain)
			}
			if targets.hasSend && (targets.sendEnd!=width-1 || targets.quitEnd>targets.sendStart) {
				t.Fatalf("width %d: Send overlaps Quit: %+v %q",width,targets,plain)
			}
		}
	}
}
