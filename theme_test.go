package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func writeConfigForTest(t *testing.T, content string) {
	t.Helper()

	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestDefaultThemeDefinesSemanticColors(t *testing.T) {
	theme := defaultTheme()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "hover text", got: theme.HoverTextColor, want: "#F9F871"},
		{name: "muted text", got: theme.MutedTextColor, want: "#707786"},
		{name: "mention background", got: theme.MentionBackgroundColor, want: "#3C3622"},
		{name: "selection background", got: theme.SelectionBackgroundColor, want: "#3D5275"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("%s = %q, want %q", test.name, test.got, test.want)
			}
		})
	}
}

func TestLegacyConfigUsesDefaultSemanticThemeColors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfigForTest(t, `name = "Legacy"
background = "#202020"
text_color = "#EEEEEE"
border_color = "#555555"
accent_color = "#7C5CFC"
`)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load legacy config: %v", err)
	}

	if cfg.Theme.Background != "#202020" {
		t.Fatalf("legacy background = %q, want #202020", cfg.Theme.Background)
	}
	if cfg.Theme.HoverTextColor != "#F9F871" {
		t.Fatalf("legacy hover text = %q, want default #F9F871", cfg.Theme.HoverTextColor)
	}
	if cfg.Theme.MutedTextColor != "#707786" {
		t.Fatalf("legacy muted text = %q, want default #707786", cfg.Theme.MutedTextColor)
	}
	if cfg.Theme.MentionBackgroundColor != "#3C3622" {
		t.Fatalf("legacy mention background = %q, want default #3C3622", cfg.Theme.MentionBackgroundColor)
	}
}

func TestConfigLoadsSemanticThemeColors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfigForTest(t, `hover_text_color = "#F0F000"
muted_text_color = "#777777"
mention_background_color = "#353020"
selection_background_color = "#405060"
`)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config with theme colors: %v", err)
	}

	if cfg.Theme.HoverTextColor != "#F0F000" {
		t.Fatalf("hover text = %q, want #F0F000", cfg.Theme.HoverTextColor)
	}
	if cfg.Theme.MutedTextColor != "#777777" {
		t.Fatalf("muted text = %q, want #777777", cfg.Theme.MutedTextColor)
	}
	if cfg.Theme.MentionBackgroundColor != "#353020" {
		t.Fatalf("mention background = %q, want #353020", cfg.Theme.MentionBackgroundColor)
	}
	if cfg.Theme.SelectionBackgroundColor != "#405060" {
		t.Fatalf("selection background = %q, want #405060", cfg.Theme.SelectionBackgroundColor)
	}
}

func TestSaveConfigRoundTripsThemeColors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := defaultConfig()
	cfg.Theme.HoverTextColor = "#F0F000"
	cfg.Theme.MutedTextColor = "#777777"
	cfg.Theme.MentionBackgroundColor = "#353020"
	cfg.Theme.SelectionBackgroundColor = "#405060"

	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	if loaded.Theme.HoverTextColor != cfg.Theme.HoverTextColor {
		t.Fatalf("saved hover text = %q, want %q", loaded.Theme.HoverTextColor, cfg.Theme.HoverTextColor)
	}
	if loaded.Theme.MutedTextColor != cfg.Theme.MutedTextColor {
		t.Fatalf("saved muted text = %q, want %q", loaded.Theme.MutedTextColor, cfg.Theme.MutedTextColor)
	}
	if loaded.Theme.MentionBackgroundColor != cfg.Theme.MentionBackgroundColor {
		t.Fatalf("saved mention background = %q, want %q", loaded.Theme.MentionBackgroundColor, cfg.Theme.MentionBackgroundColor)
	}
	if loaded.Theme.SelectionBackgroundColor != cfg.Theme.SelectionBackgroundColor {
		t.Fatalf("saved selection background = %q, want %q", loaded.Theme.SelectionBackgroundColor, cfg.Theme.SelectionBackgroundColor)
	}
}

func TestLoadConfigCreatesDefaultThemePack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	info, err := os.Stat(themesDir())
	if err != nil {
		t.Fatalf("themes directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("themes path is not a directory")
	}

	data, err := os.ReadFile(defaultThemePackPath())
	if err != nil {
		t.Fatalf("read generated default theme pack: %v", err)
	}
	var pack ThemePack
	if _, err := toml.Decode(string(data), &pack); err != nil {
		t.Fatalf("decode generated default theme pack: %v", err)
	}
	if pack.SchemaVersion != themePackSchemaVersion {
		t.Fatalf("schema version = %d, want %d", pack.SchemaVersion, themePackSchemaVersion)
	}
	if pack.PackName != "Zealot defaults" {
		t.Fatalf("pack name = %q, want Zealot defaults", pack.PackName)
	}
	if len(pack.Themes) != 10 {
		t.Fatalf("default pack contains %d themes, want 10 including Arcade", len(pack.Themes))
	}
	if pack.Themes[0].ID != "zealot" || pack.Themes[0].Name != "Zealot" {
		t.Fatalf("default theme identity = (%q, %q), want (zealot, Zealot)", pack.Themes[0].ID, pack.Themes[0].Name)
	}
	if pack.Themes[1].ID != "autumn" || pack.Themes[1].Name != "Autumn" {
		t.Fatalf("second default theme identity = (%q, %q), want (autumn, Autumn)", pack.Themes[1].ID, pack.Themes[1].Name)
	}
	if pack.Themes[2].ID != "limetime" || pack.Themes[2].Name != "Limetime" {
		t.Fatalf("third default theme identity = (%q, %q), want (limetime, Limetime)", pack.Themes[2].ID, pack.Themes[2].Name)
	}
	limetime := pack.Themes[2]
	if limetime.Description != "Heavy and high contrast. Based around the accent color #AEFF00" {
		t.Fatalf("Limetime description changed: %q", limetime.Description)
	}
	wantLimetime := Theme{
		Background: "#0E1009",
		TextColor: "#E6E6E6",
		BorderColor: "#2A3416",
		AccentColor: "#AEFF00",
		HoverTextColor: "#F9F871",
		MutedTextColor: "#869763",
		MentionBackgroundColor: "#191D0F",
		SelectionBackgroundColor: "#007000",
	}
	if limetime.Colors != wantLimetime {
		t.Fatalf("Limetime colors = %+v, want %+v", limetime.Colors, wantLimetime)
	}
	phosphor := pack.Themes[3]
	if phosphor.ID != "green-phosphor" || phosphor.Name != "Green Phosphor" {
		t.Fatalf("fourth built-in theme = (%q, %q), want Green Phosphor", phosphor.ID, phosphor.Name)
	}
	if phosphor.Description != "Classic monochrome CRT terminal glow, with every interface color rendered in shades of phosphor green." {
		t.Fatalf("Green Phosphor description changed: %q", phosphor.Description)
	}
	wantPhosphor := Theme{
		Background: "#030703",
		TextColor: "#79DC4B",
		BorderColor: "#173C1A",
		AccentColor: "#69F02E",
		HoverTextColor: "#A0F569",
		MutedTextColor: "#3F8041",
		MentionBackgroundColor: "#0A230D",
		SelectionBackgroundColor: "#1C5B22",
	}
	if phosphor.Colors != wantPhosphor {
		t.Fatalf("Green Phosphor palette = %+v, want %+v", phosphor.Colors, wantPhosphor)
	}
	fbi := pack.Themes[4]
	if fbi.ID != "fbi" || fbi.Name != "FBI" {
		t.Fatalf("fifth built-in theme = (%q, %q), want FBI", fbi.ID, fbi.Name)
	}
	if fbi.Description != "Deep federal blue, seal-inspired gold, and ceremonial red and green accents." {
		t.Fatalf("FBI description changed: %q", fbi.Description)
	}
	wantFBI := Theme{
		Background: "#07366B",
		TextColor: "#F5F3E9",
		BorderColor: "#AC9150",
		AccentColor: "#FFD348",
		HoverTextColor: "#FF8580",
		MutedTextColor: "#B5C9DF",
		MentionBackgroundColor: "#723B47",
		SelectionBackgroundColor: "#27654C",
	}
	if fbi.Colors != wantFBI {
		t.Fatalf("FBI palette = %+v, want %+v", fbi.Colors, wantFBI)
	}
	putty := pack.Themes[5]
	if putty.ID != "putty-classic" || putty.Name != "PuTTY Classic" {
		t.Fatalf("sixth built-in theme = (%q, %q), want PuTTY Classic", putty.ID, putty.Name)
	}
	if putty.Description != "Classic PuTTY ANSI terminal colors on black, with bright yellow, blue and green plus cyan, magenta and red." {
		t.Fatalf("PuTTY Classic description changed: %q", putty.Description)
	}
	wantPuTTY := Theme{
		Background:               "#000000",
		TextColor:                "#BBBBBB",
		BorderColor:              "#5555FF",
		AccentColor:              "#FFFF55",
		HoverTextColor:           "#55FF55",
		MutedTextColor:           "#00BBBB",
		MentionBackgroundColor:   "#BB00BB",
		SelectionBackgroundColor: "#BB0000",
	}
	if putty.Colors != wantPuTTY {
		t.Fatalf("PuTTY Classic palette = %+v, want %+v", putty.Colors, wantPuTTY)
	}
	for _, test := range []struct {
		index int
		id string
		name string
		description string
		colors Theme
	}{
		{
			index: 6, id: "darkroom", name: "Darkroom",
			description: "Dim red-on-red tones inspired by photographic darkrooms under a safelight.",
			colors: Theme{
				Background: "#120506",
				TextColor: "#D68A8A",
				BorderColor: "#542024",
				AccentColor: "#FF5555",
				HoverTextColor: "#FF9A9A",
				MutedTextColor: "#955054",
				MentionBackgroundColor: "#2D0C10",
				SelectionBackgroundColor: "#71252A",
			},
		},
		{
			index: 7, id: "blackout", name: "Blackout",
			description: "Pure black with uncompromising red interface highlights, a high-contrast companion to Darkroom.",
			colors: Theme{
				Background: "#000000",
				TextColor: "#DA6B6B",
				BorderColor: "#481111",
				AccentColor: "#FF3030",
				HoverTextColor: "#FF7070",
				MutedTextColor: "#8F3838",
				MentionBackgroundColor: "#240000",
				SelectionBackgroundColor: "#690909",
			},
		},
		{
			index: 8, id: "glados", name: "GladOS",
			description: "Amber-orange phosphor on near-black, inspired by GLaDOS's retro computer terminal credits in Portal.",
			colors: Theme{
				Background: "#080502",
				TextColor: "#E9A54D",
				BorderColor: "#713F13",
				AccentColor: "#FF9900",
				HoverTextColor: "#FFD079",
				MutedTextColor: "#945822",
				MentionBackgroundColor: "#261305",
				SelectionBackgroundColor: "#713706",
			},
		},
		{
			index: 9, id: "arcade", name: "Arcade",
			description: "Vibrant 90's arcade colors complete with glowsticks!",
			colors: Theme{
				Background: "#0B061A",
				TextColor: "#E3DCFF",
				BorderColor: "#8438D6",
				AccentColor: "#FF39B8",
				HoverTextColor: "#36F5FF",
				MutedTextColor: "#9178B5",
				MentionBackgroundColor: "#391152",
				SelectionBackgroundColor: "#287A39",
			},
		},
	} {
		preset := pack.Themes[test.index]
		if preset.ID != test.id || preset.Name != test.name || preset.Description != test.description || preset.Colors != test.colors {
			t.Errorf("%s built-in preset differs from expected: %+v", test.name, preset)
		}
	}
	if pack.Themes[0].Colors != cfg.Theme {
		t.Fatalf("default pack colors do not match the active default palette: got %+v, want %+v", pack.Themes[0].Colors, cfg.Theme)
	}
}

func TestEnsureThemeLibraryDoesNotOverwriteDefaultPack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(themesDir(), 0700); err != nil {
		t.Fatalf("create themes directory: %v", err)
	}
	const existing = "# user's customized defaults\n"
	if err := os.WriteFile(defaultThemePackPath(), []byte(existing), 0600); err != nil {
		t.Fatalf("write existing default pack: %v", err)
	}

	if _, err := loadConfig(); err != nil {
		t.Fatalf("load config with existing theme pack: %v", err)
	}
	got, err := os.ReadFile(defaultThemePackPath())
	if err != nil {
		t.Fatalf("read existing theme pack: %v", err)
	}
	if string(got) != existing {
		t.Fatalf("existing default pack was overwritten: got %q", string(got))
	}
}

func TestSaveAndLoadThemePack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {
		t.Fatalf("load config: %v", err)
	}

	custom := ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName:      "Community set",
		Themes: []ThemePreset{{
			ID:     "midnight-blue",
			Name:   "Midnight Blue",
			Colors: defaultTheme(),
		}},
	}
	if err := saveThemePack("community.toml", custom); err != nil {
		t.Fatalf("save custom theme pack: %v", err)
	}

	packs, err := loadThemePacks()
	if err != nil {
		t.Fatalf("load theme packs: %v", err)
	}
	if len(packs) != 2 {
		t.Fatalf("loaded %d packs, want default and community packs", len(packs))
	}
	found := false
	for _, pack := range packs {
		if pack.FileName == "community.toml" {
			found = true
			if pack.PackName != custom.PackName || len(pack.Themes) != 1 || pack.Themes[0].Name != "Midnight Blue" {
				t.Fatalf("loaded custom pack differs from saved pack: %+v", pack)
			}
		}
	}
	if !found {
		t.Fatalf("custom theme pack was not discovered")
	}
}

func TestSaveThemePackDoesNotOverwriteExistingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {
		t.Fatalf("load config: %v", err)
	}

	const original = "pack_name = \"Keep me\"\n"
	path := filepath.Join(themesDir(), "custom.toml")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatalf("write existing pack: %v", err)
	}
	pack := ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName: "Replacement",
		Themes: []ThemePreset{{ID: "replacement", Name: "Replacement", Colors: defaultTheme()}},
	}
	if err := saveThemePack("custom.toml", pack); err == nil {
		t.Fatalf("saveThemePack() overwrote an existing pack")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing pack: %v", err)
	}
	if string(got) != original {
		t.Fatalf("existing pack changed: got %q, want %q", got, original)
	}
}

func TestNormalizeUserColorAcceptsOptionalHash(t *testing.T) {
	for input, want := range map[string]string{
		"ff8c2a": "#FF8C2A",
		"#ff8c2a": "#FF8C2A",
		"255": "255",
	} {
		got, ok := normalizeUserColor(input)
		if !ok || got != want {
			t.Errorf("normalizeUserColor(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
	for _, input := range []string{"GGGGGG", "#12345", "256"} {
		if got, ok := normalizeUserColor(input); ok {
			t.Errorf("normalizeUserColor(%q) unexpectedly accepted %q", input, got)
		}
	}
}

func TestValidateThemePackRejectsInvalidColor(t *testing.T) {
	pack := defaultThemePack()
	pack.Themes[0].Colors.SelectionBackgroundColor = "not-a-color"
	if err := validateThemePack(pack); err == nil {
		t.Fatalf("validateThemePack() accepted an invalid selection color")
	}
}

func TestLoadThemePacksRejectsUnsupportedSchema(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := loadConfig(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	invalid := "schema_version = 99\npack_name = \"Future\"\n"
	if err := os.WriteFile(filepath.Join(themesDir(), "future.toml"), []byte(invalid), 0600); err != nil {
		t.Fatalf("write future pack: %v", err)
	}
	if _, err := loadThemePacks(); err == nil {
		t.Fatalf("loadThemePacks() accepted an unsupported schema version")
	}
}

func TestLoadConfigAppendsMissingThemeColorsWithoutOverwritingCustomColors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const original = `name = "Legacy"
background = "#202020"
text_color = "#EEEEEE"
border_color = "#555555"
accent_color = "#7C5CFC"
hover_text_color = "#ABCDEF"
# keep this user's comment
custom_setting = "keep"
`
	writeConfigForTest(t, original)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load legacy config: %v", err)
	}
	if cfg.Theme.HoverTextColor != "#ABCDEF" {
		t.Fatalf("custom hover color = %q, want #ABCDEF", cfg.Theme.HoverTextColor)
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"# keep this user's comment",
		"custom_setting = \"keep\"",
		"hover_text_color = \"#ABCDEF\"",
		"muted_text_color = \"#707786\"",
		"mention_background_color = \"#3C3622\"",
		"selection_background_color = \"#3D5275\"",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("migrated config does not contain %q", want)
		}
	}
}

func TestApplyThemePresetPersistsPaletteAndPreservesUserConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := defaultConfig()
	cfg.Name = "Tester"
	cfg.UserColor = "#123456"
	writeConfigForTest(t, "# Keep this config comment\nname = \"Tester\" # identity note\nuser_color = \"#123456\" # color note\n# Keep this palette comment\nbackground = \"#111318\" # background note\ncustom_setting = \"untouched\"\n")
	preset := ThemePreset{
		ID: "midnight",
		Name: "Midnight",
		Colors: Theme{
			Background: "#010203",
			TextColor: "#E0E0E0",
			BorderColor: "#303030",
			AccentColor: "#7788FF",
			HoverTextColor: "#FFFF00",
			MutedTextColor: "#808080",
			MentionBackgroundColor: "#332211",
			SelectionBackgroundColor: "#445566",
		},
	}

	if err := applyThemePreset(&cfg, preset); err != nil {
		t.Fatalf("apply theme preset: %v", err)
	}
	if cfg.Theme != preset.Colors {
		t.Fatalf("in-memory active theme = %+v, want %+v", cfg.Theme, preset.Colors)
	}
	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("reload applied theme: %v", err)
	}
	if loaded.Theme != preset.Colors {
		t.Fatalf("saved active theme = %+v, want %+v", loaded.Theme, preset.Colors)
	}
	if loaded.Name != "Tester" || loaded.UserColor != "#123456" {
		t.Fatalf("applying a theme changed user identity settings: %+v", loaded)
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read config after applying theme: %v", err)
	}
	for _, comment := range []string{"# Keep this config comment", "# identity note", "# color note", "# Keep this palette comment", "# background note", "custom_setting = \"untouched\""} {
		if !strings.Contains(string(data), comment) {
			t.Errorf("applying a theme removed %q from config", comment)
		}
	}
}

func TestSaveConfigPreservesCommentsFormattingAndCustomSettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const original = "# Keep this header\r\n" +
		"name   = \"Before\"   # preserve the name note\r\n" +
		"user_color = \"#123456\" # preserve the color note\r\n" +
		"adaptive_input = false # legacy key stays in place\r\n" +
		"input_max_percent=70\r\n" +
		"# Palette comment stays here\r\n" +
		"background = \"#111318\" # keep background note\r\n" +
		"custom_setting = \"value with # inside\" # unknown setting\r\n" +
		"# Final user comment\r\n"
	writeConfigForTest(t, original)

	cfg := defaultConfig()
	cfg.Name = "After"
	cfg.UserColor = "#ABCDEF"
	cfg.AdaptiveHeight = true
	cfg.HeightMaxPercent = 42
	cfg.Theme.AccentColor = "#123ABC"

	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	got := string(data)
	wantPrefix := "# Keep this header\r\n" +
		"name   = \"After\"   # preserve the name note\r\n" +
		"user_color = \"#ABCDEF\" # preserve the color note\r\n" +
		"adaptive_input = true # legacy key stays in place\r\n" +
		"input_max_percent=42\r\n" +
		"# Palette comment stays here\r\n" +
		"background = \"#111318\" # keep background note\r\n" +
		"custom_setting = \"value with # inside\" # unknown setting\r\n" +
		"# Final user comment\r\n"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("saveConfig changed existing config layout or comments:\n--- got ---\n%s\n--- want prefix ---\n%s", got, wantPrefix)
	}
	if !strings.Contains(got, "accent_color = \"#123ABC\"") {
		t.Fatalf("updated accent color was not persisted: %s", got)
	}
	if strings.Contains(got, "adaptive_height =") || strings.Contains(got, "height_max_percent =") {
		t.Fatalf("saveConfig added modern keys alongside legacy aliases: %s", got)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("reload config with inline comments: %v", err)
	}
	if loaded.Name != cfg.Name || loaded.UserColor != cfg.UserColor || !loaded.AdaptiveHeight || loaded.HeightMaxPercent != 42 {
		t.Fatalf("reload parsed inline-comment settings incorrectly: %+v", loaded)
	}
	if loaded.Theme.AccentColor != "#123ABC" {
		t.Fatalf("reload accent color = %q, want #123ABC", loaded.Theme.AccentColor)
	}
}

func TestIdentityConfigSavesOnlyChangeRequestedValue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const original = "# User's formatting\nname   = \"Old\" # name comment\nuser_color = 12 # bare ANSI color\nadaptive_height = true # do not touch\nbackground = \"#111318\" # theme note\ncustom_setting = \"keep\"\n"
	writeConfigForTest(t, original)

	if err := saveConfigName("New"); err != nil {
		t.Fatalf("save config name: %v", err)
	}
	data, err := os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read config after name save: %v", err)
	}
	wantAfterRename := "# User's formatting\nname   = \"New\" # name comment\nuser_color = 12 # bare ANSI color\nadaptive_height = true # do not touch\nbackground = \"#111318\" # theme note\ncustom_setting = \"keep\"\n"
	if string(data) != wantAfterRename {
		t.Fatalf("rename changed unrelated config content:\n--- got ---\n%s\n--- want ---\n%s", data, wantAfterRename)
	}

	if err := saveConfigUserColor("#ABCDEF"); err != nil {
		t.Fatalf("save config color: %v", err)
	}
	data, err = os.ReadFile(configPath())
	if err != nil {
		t.Fatalf("read config after color save: %v", err)
	}
	wantAfterColor := "# User's formatting\nname   = \"New\" # name comment\nuser_color = \"#ABCDEF\" # bare ANSI color\nadaptive_height = true # do not touch\nbackground = \"#111318\" # theme note\ncustom_setting = \"keep\"\n"
	if string(data) != wantAfterColor {
		t.Fatalf("color command changed unrelated config content:\n--- got ---\n%s\n--- want ---\n%s", data, wantAfterColor)
	}
}

func TestExistingDefaultPackGainsNewBuiltInsWithoutChangingExistingPresets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(themesDir(), 0700); err != nil {
		t.Fatalf("create themes directory: %v", err)
	}
	legacy := defaultThemePack()
	legacy.Themes = legacy.Themes[:2]
	legacy.Themes[0].Colors.AccentColor = "#123ABC"
	data, err := encodeThemePack(legacy)
	if err != nil {
		t.Fatalf("encode existing default pack: %v", err)
	}
	data = append([]byte("# preserve this comment\n"), data...)
	if err := os.WriteFile(defaultThemePackPath(), data, 0600); err != nil {
		t.Fatalf("write existing default pack: %v", err)
	}
	if err := ensureThemeLibrary(); err != nil {
		t.Fatalf("append missing built-in: %v", err)
	}
	got, err := os.ReadFile(defaultThemePackPath())
	if err != nil {
		t.Fatalf("read migrated default pack: %v", err)
	}
	if !strings.HasPrefix(string(got), string(data)) {
		t.Fatal("existing default themes content changed instead of appending")
	}
	var pack ThemePack
	if _, err := toml.Decode(string(got), &pack); err != nil {
		t.Fatalf("decode updated default pack: %v", err)
	}
	if len(pack.Themes) != 10 || pack.Themes[2].ID != "limetime" || pack.Themes[3].ID != "green-phosphor" || pack.Themes[4].ID != "fbi" || pack.Themes[5].ID != "putty-classic" || pack.Themes[6].ID != "darkroom" || pack.Themes[7].ID != "blackout" || pack.Themes[8].ID != "glados" || pack.Themes[9].ID != "arcade" {
		t.Fatalf("missing new built-in presets after migration: %+v", pack.Themes)
	}
	if pack.Themes[0].Colors.AccentColor != "#123ABC" {
		t.Fatal("customized Zealot preset was overwritten")
	}
	if err := ensureThemeLibrary(); err != nil {
		t.Fatalf("repeat theme migration: %v", err)
	}
	again, err := os.ReadFile(defaultThemePackPath())
	if err != nil {
		t.Fatalf("reread default pack: %v", err)
	}
	if string(again) != string(got) {
		t.Fatal("repeated migration appended duplicate themes or changed existing file")
	}
}
