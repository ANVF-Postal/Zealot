package main

import (
	"os"
	"path/filepath"
	"testing"
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
}

func TestSaveConfigRoundTripsThemeColors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := defaultConfig()
	cfg.Theme.HoverTextColor = "#F0F000"
	cfg.Theme.MutedTextColor = "#777777"
	cfg.Theme.MentionBackgroundColor = "#353020"

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
}
