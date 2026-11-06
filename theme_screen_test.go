package main

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestSplashIncludesCenteredThemesActionAfterStats(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.splashOpen = true
	rows := m.splashRows()
	if len(rows) < 2 || rows[len(rows)-2] != "" {
		t.Fatalf("splash theme action must be separated from stats by a blank line: %#v", rows)
	}
	if !strings.Contains(stripANSISequences(rows[len(rows)-1]), "[THEMES]") {
		t.Fatalf("splash does not contain the themes action: %q", rows[len(rows)-1])
	}
	button := "[THEMES]"
	wantX := 1 + max(0, (chatContentWidth(m.width)-len(button))/2)
	if !m.splashThemesButtonContains(wantX, 2+len(rows)-1) {
		t.Fatalf("centered themes action did not register its screen coordinate")
	}
}

func TestFlattenThemePacks(t *testing.T) {
	packs := []ThemePack{
		{PackName: "Built in", FileName: "default-themes.toml", Themes: []ThemePreset{{ID: "one", Name: "One", Colors: defaultTheme()}}},
		{PackName: "Community", FileName: "community.toml", Themes: []ThemePreset{{ID: "two", Name: "Two", Colors: defaultTheme()}, {ID: "three", Name: "Three", Colors: defaultTheme()}}},
	}
	entries := flattenThemePacks(packs)
	if len(entries) != 3 {
		t.Fatalf("flattened %d theme entries, want 3", len(entries))
	}
	if entries[1].PackName != "Community" || entries[2].Preset.ID != "three" {
		t.Fatalf("flattening lost theme pack context or ordering: %+v", entries)
	}
}

func TestThemeBrowserShowsPaletteDetails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	m := newMessageInteractionTestModel(100)
	m.height = 30
	m.themeEntries = flattenThemePacks([]ThemePack{defaultThemePack()})
	m.themeSelected = 0
	panel := stripANSISequences(m.renderThemeBrowser())
	for _, want := range []string{"THEMES", "Zealot", "COLOR PALETTE", "Background", "#111318", "Main text", "Composer selection", "#3D5275"} {
		if !strings.Contains(panel, want) {
			t.Errorf("theme browser does not show %q: %s", want, panel)
		}
	}
}

func TestThemeBrowserLoadsAllPackEntries(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	custom := ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName: "Community",
		Themes: []ThemePreset{{ID: "twilight", Name: "Twilight", Colors: defaultTheme()}},
	}
	if err := saveThemePack("community.toml", custom); err != nil {
		t.Fatalf("save community pack: %v", err)
	}
	m := newMessageInteractionTestModel(100)
	m.openThemeBrowser()
	if !m.themesOpen || len(m.themeEntries) != 2 {
		t.Fatalf("opening theme browser returned open=%v entries=%d, want true and 2", m.themesOpen, len(m.themeEntries))
	}
	if m.themeEntries[0].FileName != "default-themes.toml" || m.themeEntries[m.themeSelected].Preset.Name != "Zealot" {
		t.Fatalf("built-in active theme was not first and initially selected: selected=%+v entries=%+v", m.themeEntries[m.themeSelected], m.themeEntries)
	}
}

func TestThemeBrowserApplyUpdatesActiveThemeAndInputStyles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	m := newModel(cfg, "lobby", false, nil)
	m.width, m.height = 100, 30
	custom := defaultTheme()
	custom.Background = "#010203"
	custom.AccentColor = "#AABBCC"
	custom.SelectionBackgroundColor = "#223344"
	m.themeEntries = []themeListEntry{{Preset: ThemePreset{ID: "custom", Name: "Custom", Colors: custom}, PackName: "Test"}}
	m.applySelectedTheme()
	if m.cfg.Theme != custom {
		t.Fatalf("theme was not applied to model: %+v", m.cfg.Theme)
	}
	if m.input.Styles().Cursor.Color != lipgloss.Color(custom.AccentColor) {
		t.Fatalf("composer cursor color was not refreshed")
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "background = \"#010203\"") {
		t.Fatalf("applied theme was not persisted: %s", data)
	}
}

func TestThemeBrowserSelectionKeepsVisibleRowInRange(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.height = 14
	for i := 0; i < 20; i++ {
		m.themeEntries = append(m.themeEntries, themeListEntry{Preset: ThemePreset{Name: "Theme"}})
	}
	m.themeSelected = 19
	m.clampThemeSelection()
	if m.themeScroll == 0 || m.themeSelected >= m.themeScroll+m.themeVisibleRows() {
		t.Fatalf("selected theme is outside the visible list: selected=%d scroll=%d visible=%d", m.themeSelected, m.themeScroll, m.themeVisibleRows())
	}
}

func TestThemeBrowserListHitboxMapsVisibleRows(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.height = 24
	m.themeEntries = []themeListEntry{{Preset: ThemePreset{Name: "one"}}, {Preset: ThemePreset{Name: "two"}}}
	firstX := 2
	firstY := 4
	index, ok := m.themeBrowserListIndexAt(firstX, firstY)
	if !ok || index != 0 {
		t.Fatalf("first list row hitbox = (%d, %v), want (0, true)", index, ok)
	}
	if _, ok := m.themeBrowserListIndexAt(90, firstY); ok {
		t.Fatalf("right pane click was treated as a theme list selection")
	}
}



func TestThemeBrowserDividerCanBeResized(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.width, m.height = 100, 30
	left, right := m.themeBrowserColumnWidths()
	if left+1+right != chatInnerWidth(m.width) {
		t.Fatalf("theme columns and divider use %d cells, want %d", left+1+right, chatInnerWidth(m.width))
	}
	initial := m.themeBrowserDividerX()
	m.setThemeBrowserDivider(initial + 7)
	if got := m.themeBrowserDividerX(); got != initial+7 {
		t.Fatalf("divider moved to %d, want %d", got, initial+7)
	}
}


func TestThemeBrowserAllowsComposerTyping(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.width, m.height = 100, 30
	m.splashOpen = true
	m.themesOpen = true
	updated, _ := m.Update(tea.KeyPressMsg{Text: "h"})
	next := updated.(model)
	if next.input.Value() != "h" {
		t.Fatalf("theme browser swallowed composer input: got %q, want %q", next.input.Value(), "h")
	}
}

func TestThemeBrowserFooterKeepsComposerActions(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.width, m.height = 100, 30
	m.splashOpen = true
	m.themesOpen = true
	line, targets := m.renderStatusBar(false, false)
	plain := stripANSISequences(line)
	for _, want := range []string{"PgUp", "PgDn", "Ctrl+Enter: apply", "Esc: back", "[Send]", "Quit"} {
		if !strings.Contains(plain, want) {
			t.Errorf("theme footer is missing %q: %s", want, plain)
		}
	}
	if !targets.hasSend {
		t.Fatal("theme browser footer does not expose the send action")
	}
}

func TestBrowsingThemeOnlyPreviewsUntilExplicitApply(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	original := cfg.Theme
	custom := defaultTheme()
	custom.Background = "#010203"
	pack := ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName: "Community",
		Themes: []ThemePreset{{ID: "custom", Name: "Custom", Colors: custom}},
	}
	if err := saveThemePack("community.toml", pack); err != nil {
		t.Fatalf("save community pack: %v", err)
	}

	m := newModel(cfg, "lobby", false, nil)
	m.width, m.height = 100, 30
	m.openThemeBrowser()
	if len(m.themeEntries) < 2 {
		t.Fatalf("expected built-in and custom presets, got %d", len(m.themeEntries))
	}
	// Select by the distinct palette rather than assuming directory order.
	customIndex := -1
	for i, entry := range m.themeEntries {
		if entry.Preset.Colors == custom {
			customIndex = i
			break
		}
	}
	if customIndex < 0 {
		t.Fatalf("custom preset was not loaded: %+v", m.themeEntries)
	}
	m.themeSelected = customIndex
	m.clampThemeSelection()
	if m.cfg.Theme != original {
		t.Fatalf("browsing a preset changed the active palette")
	}
	rows := strings.Join(m.themeDetailsRows(60), "\n")
	if !strings.Contains(stripANSISequences(rows), "Preview only") {
		t.Fatalf("non-active selection is not identified as a preview: %s", stripANSISequences(rows))
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read active config: %v", err)
	}
	if strings.Contains(string(data), "background = \"#010203\"") {
		t.Fatalf("browsing a preset persisted it before explicit apply")
	}
}
