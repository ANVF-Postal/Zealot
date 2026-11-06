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
	if len(pack.Themes) != 1 {
		t.Fatalf("default pack contains %d themes, want 1", len(pack.Themes))
	}
	if pack.Themes[0].ID != "zealot" || pack.Themes[0].Name != "Zealot" {
		t.Fatalf("default theme identity = (%q, %q), want (zealot, Zealot)", pack.Themes[0].ID, pack.Themes[0].Name)
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
