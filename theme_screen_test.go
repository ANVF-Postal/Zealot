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

func TestDefaultThemePackIncludesAutumn(t *testing.T) {
	pack := defaultThemePack()
	if err := validateThemePack(pack); err != nil {
		t.Fatalf("default theme pack is invalid: %v", err)
	}

	want := Theme{
		Background:               "#2D170A",
		TextColor:                "#F7E8D8",
		BorderColor:              "#7A442A",
		AccentColor:              "#FF8C2A",
		HoverTextColor:           "#FFD166",
		MutedTextColor:           "#C18B62",
		MentionBackgroundColor:   "#622F1C",
		SelectionBackgroundColor: "#92451F",
	}
	for _, preset := range pack.Themes {
		if preset.ID == "autumn" {
			if preset.Name != "Autumn" || preset.Colors != want {
				t.Fatalf("Autumn preset = %+v, want palette %+v", preset, want)
			}
			return
		}
	}
	t.Fatal("built-in theme pack does not contain Autumn")
}

func TestEnsureThemeLibraryAddsMissingBuiltInsWithoutReplacingPack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := defaultThemePackPath()
	if err := os.MkdirAll(themesDir(), 0700); err != nil {
		t.Fatalf("create themes directory: %v", err)
	}
	originalPack := ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName:      "Zealot defaults",
		Themes: []ThemePreset{
			{ID: "zealot", Name: "Zealot", Colors: defaultTheme()},
			{ID: "my-custom", Name: "My customized preset", Colors: defaultTheme()},
		},
	}
	original, err := encodeThemePack(originalPack)
	if err != nil {
		t.Fatalf("encode original pack: %v", err)
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatalf("write original pack: %v", err)
	}

	if err := ensureThemeLibrary(); err != nil {
		t.Fatalf("ensure theme library: %v", err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated pack: %v", err)
	}
	if !strings.HasPrefix(string(updated), string(original)) {
		t.Fatal("existing default pack contents were rewritten instead of preserved")
	}

	packs, err := loadThemePacks()
	if err != nil {
		t.Fatalf("load migrated theme packs: %v", err)
	}
	if len(packs) != 1 || len(packs[0].Themes) != 3 {
		t.Fatalf("migrated pack contains %d packs and %d presets, want 1 pack and 3 presets", len(packs), len(packs[0].Themes))
	}
	foundAutumn, foundCustom := false, false
	for _, preset := range packs[0].Themes {
		foundAutumn = foundAutumn || preset.ID == "autumn"
		foundCustom = foundCustom || preset.ID == "my-custom"
	}
	if !foundAutumn || !foundCustom {
		t.Fatalf("migration lost a custom preset or failed to add Autumn: %+v", packs[0].Themes)
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

func TestThemeBrowserPaletteTextUsesThemeBackground(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	m := newModel(cfg, "lobby", false, nil)
	m.openThemeBrowser()
	if len(m.themeEntries) == 0 {
		t.Fatalf("opening theme browser did not load the generated default pack")
	}

	rows := m.themeDetailsRows(60)
	var backgroundRow string
	for _, row := range rows {
		if strings.Contains(stripANSISequences(row), "Background") {
			backgroundRow = row
			break
		}
	}
	if backgroundRow == "" {
		t.Fatal("background palette row was not rendered")
	}

	wantText := m.styles.app.Render(" Background           " + cfg.Theme.Background)
	if !strings.Contains(backgroundRow, wantText) {
		t.Fatalf("palette label/value do not use the theme background: %q", backgroundRow)
	}
	wantSwatch := lipgloss.NewStyle().Background(lipgloss.Color(cfg.Theme.Background)).Render("  ")
	if !strings.HasPrefix(backgroundRow, wantSwatch) {
		t.Fatalf("palette color swatch is missing its solid background: %q", backgroundRow)
	}
}

func TestThemeBrowserFillsTheChatViewport(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.width, m.height = 100, 30

	if got, want := m.themeVisibleRows()+2, m.messageHeight(); got != want {
		t.Fatalf("theme list rows plus title/separator = %d, want chat viewport height %d", got, want)
	}
	if got, want := lipgloss.Height(m.renderThemeBrowser()), m.messageHeight()+2; got != want {
		t.Fatalf("theme browser rendered %d rows, want %d including the outer border", got, want)
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
	if !m.themesOpen || len(m.themeEntries) != 3 {
		t.Fatalf("opening theme browser returned open=%v entries=%d, want true and 3", m.themesOpen, len(m.themeEntries))
	}
	if m.themeEntries[0].FileName != "default-themes.toml" || m.themeEntries[m.themeSelected].Preset.Name != "Zealot" {
		t.Fatalf("built-in active theme was not first and initially selected: selected=%+v entries=%+v", m.themeEntries[m.themeSelected], m.themeEntries)
	}
	foundAutumn := false
	for _, entry := range m.themeEntries {
		foundAutumn = foundAutumn || entry.Preset.ID == "autumn"
	}
	if !foundAutumn {
		t.Fatal("theme browser did not load Autumn from the built-in pack")
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



func TestThemeBrowserDividerHitboxIsOnlyTheDividerCell(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.width, m.height = 100, 30
	m.themesOpen = true
	x := m.themeBrowserDividerX()
	if !m.themeBrowserDividerContains(x, 3) {
		t.Fatal("divider cell did not register its drag hitbox")
	}
	if m.themeBrowserDividerContains(x-1, 3) || m.themeBrowserDividerContains(x+1, 3) {
		t.Fatal("divider drag hitbox extends into the adjacent panes")
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
	m.connected = false // Keep the input regression test independent of a live socket.
	m.input.Focus()
	updated, _ := m.Update(tea.KeyPressMsg{Text: "h"})
	next := updated.(model)
	if next.input.Value() != "h" {
		t.Fatalf("theme browser swallowed composer input: got %q, want %q", next.input.Value(), "h")
	}
}

func TestFooterRestoresQuitAndSendResizePriority(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.status = "Connected"

	line, targets := m.renderStatusBar(false, false)
	plain := stripANSISequences(line)
	if !strings.Contains(plain, "Connected • PgUp/PgDn: Scroll • Ctrl+C: Quit") {
		t.Fatalf("normal footer does not place Quit immediately after Ctrl+C: %q", plain)
	}
	if !strings.Contains(plain, "Enter: [Send]") {
		t.Fatalf("normal footer is missing the Enter send hint: %q", plain)
	}
	if !targets.hasSend || targets.sendStart != 93 || targets.sendEnd != 99 {
		t.Fatalf("wide footer did not anchor [Send] against the right inset: %+v", targets)
	}

	// The original resize contract: remove Enter first; retain Quit and Send;
	// compact Quit padding only at the physical limit; drop Send only when
	// Quit plus [Send] no longer fit. Left shortcut text is compressed last.
	for _, width := range []int{100, 72, 44, 28, 20, 16, 12, 10} {
		m.width = width
		line, targets = m.renderStatusBar(false, false)
		plain = stripANSISequences(line)
		if got := len([]rune(plain)); got != width {
			t.Fatalf("width %d produced footer width %d: %q", width, got, plain)
		}
		if targets.quitEnd <= targets.quitStart {
			t.Fatalf("width %d lost the Quit hitbox: %+v; footer=%q", width, targets, plain)
		}
		runes := []rune(plain)
		if targets.quitEnd > len(runes) || string(runes[targets.quitStart:targets.quitEnd]) != "Quit" {
			t.Fatalf("width %d: Quit hitbox does not cover its button: %+v; footer=%q", width, targets, plain)
		}
		if targets.hasSend {
			if targets.sendEnd != width-1 || targets.sendStart >= targets.sendEnd {
				t.Fatalf("width %d: [Send] is no longer anchored at the right inset: %+v; footer=%q", width, targets, plain)
			}
			if targets.quitEnd > targets.sendStart {
				t.Fatalf("width %d: Quit overlaps [Send]: %+v; footer=%q", width, targets, plain)
			}
		} else if width > 10 {
			t.Fatalf("width %d dropped [Send] before the minimum physical width: %q", width, plain)
		}
		if width == 44 && (strings.Contains(plain, "Enter: ") || !targets.hasSend) {
			t.Fatalf("width 44 should sacrifice Enter: before [Send]: targets=%+v footer=%q", targets, plain)
		}
		if width == 12 && (!targets.hasSend || targets.quitStart != 0) {
			t.Fatalf("width 12 should preserve both buttons with Quit padding compacted: targets=%+v footer=%q", targets, plain)
		}
		if width == 10 && targets.hasSend {
			t.Fatalf("width 10 should drop [Send] only after Quit padding is exhausted: %+v; footer=%q", targets, plain)
		}
	}
}

func TestThemeBrowserFooterKeepsComposerActions(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.width, m.height = 100, 30
	m.splashOpen = true
	m.themesOpen = true
	line, targets := m.renderStatusBar(false, false)
	plain := stripANSISequences(line)
	for _, want := range []string{"PgUp", "PgDn", "Ctrl+T: Apply • Esc: back • Ctrl+C:", "Apply", "Quit", "[Send]"} {
		if !strings.Contains(plain, want) {
			t.Errorf("theme footer is missing %q: %s", want, plain)
		}
	}
	runes := []rune(plain)
	if !targets.hasApply || targets.applyStart < 0 || targets.applyEnd > len(runes) ||
		string(runes[targets.applyStart:targets.applyEnd]) != "Apply" {
		t.Fatalf("theme footer does not expose a clickable Apply button: %+v; %s", targets, plain)
	}
	if !strings.Contains(plain, "Ctrl+T: Apply") {
		t.Fatalf("Apply should follow the Ctrl+T shortcut in the theme hint: %s", plain)
	}
	if targets.quitEnd <= targets.quitStart || targets.quitEnd > len(runes) ||
		string(runes[targets.quitStart:targets.quitEnd]) != "Quit" {
		t.Fatalf("Quit hitbox does not cover its button: %+v; %s", targets, plain)
	}
	if targets.applyEnd > targets.quitStart || targets.quitEnd > targets.sendStart {
		t.Fatalf("theme action buttons overlap: %+v; %s", targets, plain)
	}
	if strings.Count(plain, "Apply") != 1 || strings.Contains(plain, "apply") {
		t.Fatalf("theme footer should contain exactly one title-case Apply button: %s", plain)
	}
	if !targets.hasSend {
		t.Fatal("theme browser footer does not expose the send action")
	}
	if !targets.hasApply {
		t.Fatal("theme browser footer does not expose a clickable apply action")
	}
}

func TestThemeBrowserFooterQuitClickOpensConfirmation(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.width, m.height = 72, 30
	m.status = "Connected"
	m.splashOpen = true
	m.themesOpen = true
	line, targets := m.renderStatusBar(false, false)
	plain := stripANSISequences(line)
	if !strings.Contains(plain, "Ctrl+C:") || !strings.Contains(plain, "Apply") || !strings.Contains(plain, "Quit") {
		t.Fatalf("test setup did not render the theme action cluster after the Ctrl+C hint: %q", plain)
	}
	if targets.quitEnd <= targets.quitStart {
		t.Fatal("footer did not expose the clickable Quit button after the Ctrl+C hint")
	}
	updated, _ := m.Update(tea.MouseClickMsg{
		X: targets.quitStart,
		Y: m.height - 1,
		Button: tea.MouseLeft,
	})
	next := updated.(model)
	if !next.quitConfirm {
		t.Fatal("clicking Quit beside Ctrl+C did not open quit confirmation")
	}
	sureLine, sureTargets := next.renderStatusBar(false, false)
	surePlain := stripANSISequences(sureLine)
	if !strings.Contains(surePlain, "Sure?") {
		t.Fatalf("Quit confirmation did not replace the button label with Sure?: %q", surePlain)
	}
	sureRunes := []rune(surePlain)
	if sureTargets.quitEnd <= sureTargets.quitStart || sureTargets.quitEnd > len(sureRunes) ||
		string(sureRunes[sureTargets.quitStart:sureTargets.quitEnd]) != "Sure?" {
		t.Fatalf("confirmation hitbox does not cover Sure?: %+v; footer=%q", sureTargets, surePlain)
	}
	timedOut, _ := next.Update(quitConfirmTimeoutMsg{generation: next.quitConfirmGeneration})
	if timedOut.(model).quitConfirm {
		t.Fatal("Quit confirmation did not reset when its timeout elapsed")
	}
}

func TestThemeFooterApplyStaysClickableInButtonClusterAcrossWidths(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.themesOpen = true
	m.splashOpen = true
	m.status = "Connected"

	for _, width := range []int{100, 72, 44, 28, 20, 16, 12, 10, 8} {
		m.width = width
		line, targets := m.renderStatusBar(false, false)
		plain := stripANSISequences(line)
		runes := []rune(plain)
		if got := len(runes); got != width {
			t.Fatalf("width %d produced footer width %d: %q", width, got, plain)
		}
		if targets.hasApply {
			if targets.applyEnd <= targets.applyStart || targets.applyEnd > len(runes) ||
				string(runes[targets.applyStart:targets.applyEnd]) != "Apply" {
				t.Fatalf("width %d: Apply hitbox does not cover the visible button: %+v; footer=%q", width, targets, plain)
			}
			if targets.applyEnd > targets.quitStart {
				t.Fatalf("width %d: Apply overlaps Quit: %+v; footer=%q", width, targets, plain)
			}
			if strings.Count(plain, "Apply") != 1 {
				t.Fatalf("width %d: expected one Apply button in the action cluster: %q", width, plain)
			}
		} else if strings.Contains(plain, "Apply") {
			t.Fatalf("width %d: Apply is visible without a hitbox: %+v; footer=%q", width, targets, plain)
		}
		if strings.Contains(plain, "Quit") {
			if targets.quitEnd <= targets.quitStart || targets.quitEnd > len(runes) ||
				string(runes[targets.quitStart:targets.quitEnd]) != "Quit" {
				t.Fatalf("width %d: Quit hitbox does not cover the visible button: %+v; footer=%q", width, targets, plain)
			}
		}
		if targets.hasSend && (targets.quitEnd > targets.sendStart || targets.sendEnd != width-1) {
			t.Fatalf("width %d: button cluster overlaps Send or Send moved from the right inset: %+v; footer=%q", width, targets, plain)
		}
		if targets.hasApply && targets.applyEnd > targets.quitStart {
			t.Fatalf("width %d: Apply hitbox overlaps Quit: %+v; footer=%q", width, targets, plain)
		}
	}
}

func themeBrowserSelectAutumn(t *testing.T, m *model) Theme {
	t.Helper()
	for i, entry := range m.themeEntries {
		if entry.Preset.ID == "autumn" {
			m.themeSelected = i
			m.clampThemeSelection()
			return entry.Preset.Colors
		}
	}
	t.Fatal("Autumn preset not found in theme browser")
	return Theme{}
}

func TestThemeBrowserActionsShareOneHorizontalRow(t *testing.T) {
	m := newThemeEditorTestModel(t)
	_, rightWidth := m.themeBrowserColumnWidths()
	rows := m.themeDetailsRows(rightWidth)
	actionRow := m.themeActionButtonRowForWidth(rightWidth)
	row := stripANSISequences(rows[actionRow])
	cloneStart, cloneEnd, cloneOK := themeActionButtonBounds(rightWidth, themeCloneButtonLabel, false)
	applyStart, applyEnd, applyOK := themeActionButtonBounds(rightWidth, themeApplyButtonLabel, false)
	if !cloneOK || !applyOK || cloneEnd > applyStart {
		t.Fatalf("clean actions are not laid out horizontally: clone=(%d,%d,%v) apply=(%d,%d,%v)", cloneStart, cloneEnd, cloneOK, applyStart, applyEnd, applyOK)
	}
	if !strings.Contains(row, themeCloneButtonLabel) || !strings.Contains(row, themeApplyButtonLabel) {
		t.Fatalf("horizontal action row is missing labels: %q", row)
	}
	if cloneStart != 0 {
		t.Fatalf("action buttons are not anchored left: Clone starts at column %d", cloneStart)
	}

	m.themeDirty = true
	rows = m.themeDetailsRows(rightWidth)
	row = stripANSISequences(rows[actionRow])
	cloneStart, cloneEnd, cloneOK = themeActionButtonBounds(rightWidth, themeCloneButtonLabel, true)
	saveStart, saveEnd, saveOK := themeActionButtonBounds(rightWidth, themeSaveButtonLabel, true)
	applyStart, applyEnd, applyOK = themeActionButtonBounds(rightWidth, themeApplyButtonLabel, true)
	if !cloneOK || !saveOK || !applyOK || cloneEnd > saveStart || saveEnd > applyStart {
		t.Fatalf("dirty actions are not laid out in order on one row: clone=(%d,%d,%v) save=(%d,%d,%v) apply=(%d,%d,%v)", cloneStart, cloneEnd, cloneOK, saveStart, saveEnd, saveOK, applyStart, applyEnd, applyOK)
	}
	for _, label := range []string{themeCloneButtonLabel, themeSaveButtonLabel, themeApplyButtonLabel} {
		if !strings.Contains(row, label) {
			t.Errorf("dirty horizontal action row is missing %s: %q", label, row)
		}
	}
}

func TestThemeBrowserWrapsOnlyDescriptionContent(t *testing.T) {
	m := newThemeEditorTestModel(t)
	entry, _ := m.selectedThemeEntry()
	entry.Preset.Description = "A very long description that should wrap within the details pane while preserving the one row per color palette layout."
	m.themeEntries[m.themeSelected] = entry

	_, width := m.themeBrowserColumnWidths()
	rows := m.themeDetailsRows(width)
	descriptionStart := m.themeDescriptionTextStartRow()
	if len(m.themeDescriptionLines(width)) < 2 {
		t.Fatal("long description did not wrap")
	}
	for i := 0; i < themePaletteColorCount; i++ {
		if lipglossWidthForThemeTest(stripANSISequences(rows[themePaletteFirstRow+i])) > width {
			t.Fatalf("palette row %d exceeded pane width", i)
		}
	}
	if m.themeActionButtonRowForWidth(width) <= descriptionStart+1 {
		t.Fatal("action row did not move below the wrapped description")
	}
}

func lipglossWidthForThemeTest(value string) int {
	return len([]rune(value))
}

func TestThemeBrowserCtrlTShortcutAppliesSelectedTheme(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "enhanced key with Ctrl modifier", key: tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}},
		{name: "legacy control byte", key: tea.KeyPressMsg{Code: 20}},
		{name: "control byte in text", key: tea.KeyPressMsg{Text: string(rune(20))}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			cfg, err := loadConfig()
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			m := newModel(cfg, "lobby", false, nil)
			m.width, m.height = 100, 30
			m.openThemeBrowser()
			want := themeBrowserSelectAutumn(t, &m)

			updated, _ := m.Update(test.key)
			next := updated.(model)
			if next.cfg.Theme != want {
				t.Fatalf("Ctrl+T did not apply selected theme: got %+v, want %+v", next.cfg.Theme, want)
			}
		})
	}
}

func TestThemeBrowserF2ShortcutAppliesSelectedTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	m := newModel(cfg, "lobby", false, nil)
	m.width, m.height = 100, 30
	m.openThemeBrowser()
	want := themeBrowserSelectAutumn(t, &m)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	next := updated.(model)
	if next.cfg.Theme != want {
		t.Fatalf("F2 did not apply selected theme: got %+v, want %+v", next.cfg.Theme, want)
	}
}

func TestThemeBrowserApplyButtonClickAppliesSelectedTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	m := newModel(cfg, "lobby", false, nil)
	m.width, m.height = 100, 30
	m.openThemeBrowser()
	want := themeBrowserSelectAutumn(t, &m)
	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	details := stripANSISequences(strings.Join(m.themeDetailsRows(rightWidth), "\n"))
	if !strings.Contains(details, "[APPLY]") {
		t.Fatal("right pane does not render its apply action")
	}
	start, end, ok := themeActionButtonBounds(rightWidth, themeApplyButtonLabel, false)
	if !ok {
		t.Fatal("right pane did not lay out the Apply action")
	}
	x := m.themeBrowserContentStartX() + leftWidth + 1 + (start+end)/2
	y := 2 + m.themeActionButtonRowForWidth(rightWidth)
	if !m.themeApplyButtonContains(x, y) {
		t.Fatalf("calculated apply button center (%d,%d) is not inside its hitbox", x, y)
	}
	updated, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	next := updated.(model)
	if next.cfg.Theme != want {
		t.Fatalf("clicking right-pane apply button did not apply theme: got %+v, want %+v", next.cfg.Theme, want)
	}
}

func TestThemeBrowserFooterApplyClickAppliesSelectedTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	m := newModel(cfg, "lobby", false, nil)
	m.width, m.height = 72, 30
	m.openThemeBrowser()
	want := themeBrowserSelectAutumn(t, &m)
	_, targets := m.renderStatusBar(false, false)
	if !targets.hasApply {
		t.Fatalf("resized footer did not expose the Apply button hitbox: %+v", targets)
	}
	updated, _ := m.Update(tea.MouseClickMsg{X: targets.applyStart, Y: m.height - 1, Button: tea.MouseLeft})
	next := updated.(model)
	if next.cfg.Theme != want {
		t.Fatalf("clicking footer apply action did not apply theme: got %+v, want %+v", next.cfg.Theme, want)
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


func TestThemeFooterProgressiveShortcutClipping(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.themesOpen = true
	m.splashOpen = true
	m.status = "Connected"

	// The optional Enter hint yields before any shortcut text.
	m.width = 100
	wide, _ := m.renderStatusBar(false, false)
	if got := stripANSISequences(wide); !strings.Contains(got, "Ctrl+T: Apply • Esc: back • Ctrl+C: Quit") ||
		!strings.Contains(got, "Enter: [Send]") {
		t.Fatalf("wide theme footer lost its full hints: %q", got)
	}

	m.width = 44
	medium, mediumTargets := m.renderStatusBar(false, false)
	if got := stripANSISequences(medium); !strings.Contains(got, "PgUp/PgDn:") ||
		!strings.Contains(got, "Apply Quit [Send]") ||
		strings.Contains(got, "Enter:") {
		t.Fatalf("medium footer should clip Ctrl+T hint before button labels: %q", got)
	}
	if !mediumTargets.hasPageScroll || !mediumTargets.hasApply || !mediumTargets.hasSend {
		t.Fatal("medium footer lost a clickable button prematurely")
	}

	m.width = 28
	narrow, narrowTargets := m.renderStatusBar(false, false)
	if got := stripANSISequences(narrow); !strings.Contains(got, "PgUp/PgDn Apply Quit [Send]") {
		t.Fatalf("narrow footer must keep the full page buttons: %q", got)
	}
	if !narrowTargets.hasPageScroll {
		t.Fatal("page control hitboxes disappeared while the labels fit")
	}

	m.width = 20
	tiny, tinyTargets := m.renderStatusBar(false, false)
	if got := stripANSISequences(tiny); strings.Contains(got, "PgUp") ||
		!strings.Contains(got, "Apply Quit [Send]") {
		t.Fatalf("tiny footer must drop page controls before action buttons: %q", got)
	}
	if tinyTargets.hasPageScroll || !tinyTargets.hasApply || !tinyTargets.hasSend {
		t.Fatal("tiny footer did not preserve the remaining clickable actions")
	}
}

func TestThemeFooterResponsiveActionsRemainOrdered(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.themesOpen = true
	m.splashOpen = true
	m.status = "Connected"

	for _, width := range []int{100, 72, 44, 28, 20, 16, 12} {
		m.width = width
		line, targets := m.renderStatusBar(false, false)
		plain := stripANSISequences(line)
		cells := []rune(plain)
		if len(cells) != width {
			t.Fatalf("width %d: rendered %d cells: %q", width, len(cells), plain)
		}
		if targets.hasApply {
			if targets.applyStart < 0 || targets.applyEnd > width ||
				string(cells[targets.applyStart:targets.applyEnd]) != "Apply" {
				t.Fatalf("width %d: Apply text and mouse target disagree: %+v %q", width, targets, plain)
			}
			if targets.hasPageScroll && targets.pgDnEnd >= targets.applyStart {
				t.Fatalf("width %d: page controls collide with Apply: %+v %q", width, targets, plain)
			}
			if targets.applyEnd >= targets.quitStart {
				t.Fatalf("width %d: Apply must stay before Quit: %+v %q", width, targets, plain)
			}
		}
		if targets.hasPageScroll {
			if string(cells[targets.pgUpStart:targets.pgUpEnd]) != "PgUp" ||
				string(cells[targets.pgDnStart:targets.pgDnEnd]) != "PgDn" {
				t.Fatalf("width %d: page button text and hitboxes disagree: %+v %q", width, targets, plain)
			}
		}
		if width >= 28 && !targets.hasPageScroll {
			t.Fatalf("width %d: dropped clickable page controls too early: %q", width, plain)
		}
		if targets.hasSend && (targets.quitEnd > targets.sendStart || targets.sendEnd != width-1) {
			t.Fatalf("width %d: Quit and Send overlap or Send lost its right anchor: %+v %q", width, targets, plain)
		}
	}
}
