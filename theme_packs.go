package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	themePackSchemaVersion   = 1
	defaultThemePackFilename = "default-themes.toml"
)

// ThemePack is a shareable collection of complete theme presets. FileName is
// populated when a pack is loaded and is deliberately excluded from TOML.
type ThemePack struct {
	SchemaVersion int           `toml:"schema_version"`
	PackName      string        `toml:"pack_name"`
	Themes        []ThemePreset `toml:"themes"`
	FileName      string        `toml:"-"`
}

// ThemePreset is one named palette inside a theme pack.
type ThemePreset struct {
	ID     string `toml:"id"`
	Name   string `toml:"name"`
	Colors Theme  `toml:"colors"`
}

func defaultThemePack() ThemePack {
	return ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName:      "Zealot defaults",
		Themes: []ThemePreset{{
			ID:     "zealot",
			Name:   "Zealot",
			Colors: defaultTheme(),
		}},
	}
}

func themesDir() string {
	return filepath.Join(filepath.Dir(configPath()), "themes")
}

func defaultThemePackPath() string {
	return filepath.Join(themesDir(), defaultThemePackFilename)
}

// ensureThemeLibrary creates the user theme directory and installs the
// built-in pack only when it is absent. Existing user files are never replaced.
func ensureThemeLibrary() error {
	if err := os.MkdirAll(themesDir(), 0700); err != nil {
		return fmt.Errorf("create themes directory: %w", err)
	}

	content, err := encodeThemePack(defaultThemePack())
	if err != nil {
		return fmt.Errorf("encode default theme pack: %w", err)
	}
	file, err := os.OpenFile(defaultThemePackPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create default theme pack: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(defaultThemePackPath())
		return fmt.Errorf("write default theme pack: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(defaultThemePackPath())
		return fmt.Errorf("close default theme pack: %w", err)
	}
	return nil
}

// loadThemePacks reads and validates every TOML theme pack in the user's
// themes directory. os.ReadDir returns entries in filename order.
func loadThemePacks() ([]ThemePack, error) {
	if err := ensureThemeLibrary(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(themesDir())
	if err != nil {
		return nil, fmt.Errorf("read themes directory: %w", err)
	}

	packs := make([]ThemePack, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".toml") {
			continue
		}
		path := filepath.Join(themesDir(), entry.Name())
		var pack ThemePack
		if _, err := toml.DecodeFile(path, &pack); err != nil {
			return nil, fmt.Errorf("decode theme pack %q: %w", entry.Name(), err)
		}
		if err := validateThemePack(pack); err != nil {
			return nil, fmt.Errorf("validate theme pack %q: %w", entry.Name(), err)
		}
		pack.FileName = entry.Name()
		packs = append(packs, pack)
	}
	return packs, nil
}

// saveThemePack writes a user-created pack. The built-in default pack is
// reserved so the application never overwrites the presets shipped with Zealot.
func saveThemePack(filename string, pack ThemePack) error {
	if filename == "" || filepath.Base(filename) != filename || !strings.EqualFold(filepath.Ext(filename), ".toml") {
		return fmt.Errorf("theme pack filename must be a plain .toml filename")
	}
	if strings.EqualFold(filename, defaultThemePackFilename) {
		return fmt.Errorf("%s is reserved for Zealot's built-in themes", defaultThemePackFilename)
	}
	if err := validateThemePack(pack); err != nil {
		return err
	}
	if err := os.MkdirAll(themesDir(), 0700); err != nil {
		return fmt.Errorf("create themes directory: %w", err)
	}
	content, err := encodeThemePack(pack)
	if err != nil {
		return err
	}
	path := filepath.Join(themesDir(), filename)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("theme pack %q already exists; refusing to overwrite it", filename)
	}
	if err != nil {
		return fmt.Errorf("create theme pack %q: %w", filename, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write theme pack %q: %w", filename, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close theme pack %q: %w", filename, err)
	}
	return nil
}

func encodeThemePack(pack ThemePack) ([]byte, error) {
	pack.FileName = ""
	var content bytes.Buffer
	if err := toml.NewEncoder(&content).Encode(pack); err != nil {
		return nil, fmt.Errorf("encode theme pack: %w", err)
	}
	return content.Bytes(), nil
}

// applyThemePreset persists a preset as the active palette while preserving
// the user's non-theme configuration. The caller can then refresh live UI
// styles from the updated Config.
func applyThemePreset(cfg *Config, preset ThemePreset) error {
	if cfg == nil {
		return fmt.Errorf("cannot apply a theme to a nil config")
	}
	if err := validateThemeColors(preset.Colors); err != nil {
		return fmt.Errorf("cannot apply theme %q: %w", preset.ID, err)
	}

	updated := *cfg
	updated.Theme = preset.Colors
	if err := saveConfig(updated); err != nil {
		return fmt.Errorf("save active theme: %w", err)
	}
	cfg.Theme = preset.Colors
	return nil
}

func validateThemePack(pack ThemePack) error {
	if pack.SchemaVersion != themePackSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d (supported: %d)", pack.SchemaVersion, themePackSchemaVersion)
	}
	if strings.TrimSpace(pack.PackName) == "" {
		return fmt.Errorf("pack_name cannot be empty")
	}
	if len(pack.Themes) == 0 {
		return fmt.Errorf("theme pack must contain at least one theme")
	}

	seenIDs := make(map[string]struct{}, len(pack.Themes))
	for index, preset := range pack.Themes {
		if strings.TrimSpace(preset.ID) == "" {
			return fmt.Errorf("theme %d has an empty id", index+1)
		}
		if strings.TrimSpace(preset.Name) == "" {
			return fmt.Errorf("theme %q has an empty name", preset.ID)
		}
		key := strings.ToLower(strings.TrimSpace(preset.ID))
		if _, exists := seenIDs[key]; exists {
			return fmt.Errorf("duplicate theme id %q", preset.ID)
		}
		seenIDs[key] = struct{}{}

		if err := validateThemeColors(preset.Colors); err != nil {
			return fmt.Errorf("theme %q: %w", preset.ID, err)
		}
	}
	return nil
}

func validateThemeColors(theme Theme) error {
	colors := map[string]string{
		"background":                 theme.Background,
		"text_color":                 theme.TextColor,
		"border_color":               theme.BorderColor,
		"accent_color":                theme.AccentColor,
		"hover_text_color":            theme.HoverTextColor,
		"muted_text_color":            theme.MutedTextColor,
		"mention_background_color":    theme.MentionBackgroundColor,
		"selection_background_color":  theme.SelectionBackgroundColor,
	}
	for name, value := range colors {
		if !validUserColor(value) {
			return fmt.Errorf("invalid %s color %q", name, value)
		}
	}
	return nil
}
