package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newThemeEditorTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	m := newModel(cfg, "lobby", false, nil)
	m.width, m.height = 100, 30
	m.openThemeBrowser()
	if len(m.themeEntries) < 2 {
		t.Fatalf("theme browser loaded %d entries, want built-in Zealot and Autumn", len(m.themeEntries))
	}
	return m
}

func TestThemeCreateButtonAddsFreshDefaultThemeFile(t *testing.T) {
	m := newThemeEditorTestModel(t)
	leftWidth, _ := m.themeBrowserColumnWidths()
	x := m.themeBrowserContentStartX() + leftWidth - len(themeCreateButtonLabel)
	if !m.themeCreateButtonContains(x, 2) {
		t.Fatal("plus button did not register its visible hitbox")
	}

	updated, _ := m.Update(tea.MouseClickMsg{X: x, Y: 2, Button: tea.MouseLeft})
	next := updated.(model)
	entry, ok := next.selectedThemeEntry()
	if !ok {
		t.Fatal("creating a theme left no selected entry")
	}
	if entry.FileName == defaultThemePackFilename || entry.Preset.Colors != defaultTheme() {
		t.Fatalf("new theme = %+v, want a separate file with Zealot default colors", entry)
	}
	if _, err := os.Stat(filepath.Join(themesDir(), entry.FileName)); err != nil {
		t.Fatalf("new theme file was not created: %v", err)
	}
	if next.themeDirty {
		t.Fatal("fresh theme should be saved before the user edits it")
	}
}

func TestCloneBuiltInThemeCreatesIndependentFile(t *testing.T) {
	m := newThemeEditorTestModel(t)
	wantColors := themeBrowserSelectAutumn(t, &m)
	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	rightStart := m.themeBrowserContentStartX() + leftWidth + 1
	x := rightStart + max(0, (rightWidth-len(themeCloneButtonLabel)-2)/2)
	y := 2 + themeCloneButtonRow()
	if !m.themeCloneButtonContains(x, y) {
		t.Fatal("clone button did not register its visible hitbox")
	}

	updated, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	next := updated.(model)
	entry, ok := next.selectedThemeEntry()
	if !ok {
		t.Fatal("cloning a built-in theme left no selected entry")
	}
	if entry.FileName == defaultThemePackFilename {
		t.Fatal("clone still points at the protected built-in pack")
	}
	if entry.Preset.Name != "Autumn Copy" || entry.Preset.Colors != wantColors {
		t.Fatalf("cloned preset = %+v, want Autumn Copy with the original colors", entry.Preset)
	}
	data, err := os.ReadFile(filepath.Join(themesDir(), entry.FileName))
	if err != nil {
		t.Fatalf("read cloned theme file: %v", err)
	}
	pack, err := decodeThemePackBytes(data)
	if err != nil {
		t.Fatalf("decode cloned theme file: %v", err)
	}
	if len(pack.Themes) != 1 || pack.Themes[0].Colors != wantColors {
		t.Fatalf("clone file contains %+v, want exactly one copied theme", pack.Themes)
	}
	if themeEntryEditable(themeListEntry{FileName: defaultThemePackFilename}) {
		t.Fatal("built-in pack unexpectedly became editable")
	}
}

func TestThemeEditorRenameAndColorChangesSaveToRenamedFile(t *testing.T) {
	m := newThemeEditorTestModel(t)
	themeBrowserSelectAutumn(t, &m)
	m.cloneSelectedTheme()
	entry, ok := m.selectedThemeEntry()
	if !ok {
		t.Fatal("clone did not create a selected theme")
	}
	oldFilename := entry.FileName

	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	rightStart := m.themeBrowserContentStartX() + leftWidth + 1
	updated, _ := m.Update(tea.MouseClickMsg{
		X: rightStart, Y: 2, Button: tea.MouseLeft,
	})
	m = updated.(model)
	if m.themeEditField != "name" {
		t.Fatalf("clicking custom theme name opened field %q, want name", m.themeEditField)
	}
	m.input.SetValue("Burnt Orange")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if m.themeDirty != true || m.themeEditField != "" {
		t.Fatalf("confirming name edit left dirty=%v field=%q", m.themeDirty, m.themeEditField)
	}

	const accentRow = 2 + themePaletteFirstRow + 3
	updated, _ = m.Update(tea.MouseClickMsg{
		X: rightStart + 2, Y: accentRow, Button: tea.MouseLeft,
	})
	m = updated.(model)
	if m.themeEditField != "accent_color" {
		t.Fatalf("clicking Accent row opened field %q, want accent_color", m.themeEditField)
	}
	m.input.SetValue("#FFA500")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)
	if !m.themeDirty {
		t.Fatal("color edit did not expose unsaved changes")
	}
	if !strings.Contains(stripANSISequences(strings.Join(m.themeDetailsRows(rightWidth), "\n")), themeSaveButtonLabel) {
		t.Fatal("dirty theme does not show its Save button")
	}

	saveX := rightStart + max(0, (rightWidth-len(themeSaveButtonLabel)-2)/2)
	saveY := 2 + themeSaveButtonRow()
	if !m.themeSaveButtonContains(saveX, saveY) {
		t.Fatalf("Save button did not register its visible hitbox at (%d,%d)", saveX, saveY)
	}
	updated, _ = m.Update(tea.MouseClickMsg{X: saveX, Y: saveY, Button: tea.MouseLeft})
	m = updated.(model)

	saved, ok := m.selectedThemeEntry()
	if !ok {
		t.Fatal("saved theme disappeared from the picker")
	}
	if saved.Preset.Name != "Burnt Orange" || saved.Preset.Colors.AccentColor != "#FFA500" {
		t.Fatalf("saved theme = %+v, want renamed theme with new accent color", saved.Preset)
	}
	if saved.FileName != "theme-burnt-orange.toml" || saved.FileName == oldFilename {
		t.Fatalf("renamed theme filename = %q, want theme-burnt-orange.toml (was %q)", saved.FileName, oldFilename)
	}
	if m.themeDirty {
		t.Fatal("successful save left the theme marked dirty")
	}
	if _, err := os.Stat(filepath.Join(themesDir(), oldFilename)); !os.IsNotExist(err) {
		t.Fatalf("old theme file still exists after rename, stat error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(themesDir(), saved.FileName))
	if err != nil {
		t.Fatalf("read renamed theme file: %v", err)
	}
	pack, err := decodeThemePackBytes(data)
	if err != nil {
		t.Fatalf("decode renamed theme file: %v", err)
	}
	if len(pack.Themes) != 1 || pack.Themes[0].Name != "Burnt Orange" || pack.Themes[0].Colors.AccentColor != "#FFA500" {
		t.Fatalf("saved TOML pack = %+v", pack)
	}
}

func TestSavingPaletteOnlyChangeOverwritesExistingThemeFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	entry, err := createThemePresetFile(ThemePreset{Name: "Midnight", Colors: defaultTheme()})
	if err != nil {
		t.Fatalf("create theme: %v", err)
	}
	originalFilename := entry.FileName
	changed := entry.Preset
	changed.Colors.AccentColor = "#123456"

	saved, err := saveThemePresetFile(entry, changed)
	if err != nil {
		t.Fatalf("save palette update: %v", err)
	}
	if saved.FileName != originalFilename {
		t.Fatalf("palette-only save changed filename from %q to %q", originalFilename, saved.FileName)
	}
	if _, err := os.Stat(filepath.Join(themesDir(), originalFilename)); err != nil {
		t.Fatalf("original theme file was not preserved: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(themesDir(), originalFilename))
	if err != nil {
		t.Fatalf("read updated theme file: %v", err)
	}
	pack, err := decodeThemePackBytes(data)
	if err != nil {
		t.Fatalf("decode updated theme file: %v", err)
	}
	if len(pack.Themes) != 1 || pack.Themes[0].Colors.AccentColor != "#123456" {
		t.Fatalf("overwritten theme file = %+v", pack.Themes)
	}
}

func TestApplyingThemeRebuildsCachedMessageRowsImmediately(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	packet := interactionTestPacket("theme-cache-message", "Existing message")
	m := newMessageInteractionTestModel(100, packet)
	before := strings.Join(m.renderMessageLines(), "\n")

	custom := defaultTheme()
	custom.Background = "#010203"
	m.themeEntries = []themeListEntry{{Preset: ThemePreset{ID: "cache-test", Name: "Cache test", Colors: custom}, FileName: "custom.toml"}}
	m.themeSelected = 0
	m.applySelectedTheme()

	after := strings.Join(m.renderMessageLines(), "\n")
	if before == after {
		t.Fatal("applying a theme left cached message rows unchanged")
	}
	if !strings.Contains(after, "48;2;1;2;3") {
		t.Fatalf("cached message rows do not use the new background: %q", after)
	}
}

func TestApplyingDirtyCustomThemeRequiresSavingFirst(t *testing.T) {
	m := newThemeEditorTestModel(t)
	themeBrowserSelectAutumn(t, &m)
	m.cloneSelectedTheme()
	entry, ok := m.selectedThemeEntry()
	if !ok || !themeEntryEditable(entry) {
		t.Fatal("cloning the built-in theme did not create an editable theme")
	}
	originalActive := m.cfg.Theme
	m.beginThemeFieldEdit("background")
	m.input.SetValue("#010203")
	m.finishThemeFieldEdit()
	if !m.themeDirty {
		t.Fatal("editing the custom theme did not mark it dirty")
	}

	m.applySelectedTheme()
	if m.cfg.Theme != originalActive {
		t.Fatal("applying an unsaved custom theme changed the active config")
	}
	if !strings.Contains(m.themeEditError, "Save") {
		t.Fatalf("unsaved apply did not explain that Save is required: %q", m.themeEditError)
	}
}

func TestBuiltInThemePaletteIsReadOnlyButCloneIsAvailable(t *testing.T) {
	m := newThemeEditorTestModel(t)
	entry, _ := m.selectedThemeEntry()
	if themeEntryEditable(entry) {
		t.Fatal("built-in Zealot preset should be read-only")
	}
	m.beginThemeFieldEdit("name")
	if m.themeEditField != "" {
		t.Fatal("built-in theme name unexpectedly entered edit mode")
	}
	if !strings.Contains(stripANSISequences(strings.Join(m.themeDetailsRows(60), "\n")), themeCloneButtonLabel) {
		t.Fatal("built-in theme does not expose Clone")
	}
}
