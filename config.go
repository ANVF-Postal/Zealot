package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

// Theme contains the colors used by Zealot's ordinary interface. Special
// effects such as rainbow rendering and XKeyscore artwork intentionally keep
// their own palettes.
type Theme struct {
	Background               string `toml:"background"`
	TextColor                string `toml:"text_color"`
	BorderColor              string `toml:"border_color"`
	AccentColor              string `toml:"accent_color"`
	HoverTextColor           string `toml:"hover_text_color"`
	MutedTextColor           string `toml:"muted_text_color"`
	MentionBackgroundColor   string `toml:"mention_background_color"`
	SelectionBackgroundColor string `toml:"selection_background_color"`
}

type Config struct {
	Name             string
	UserColor        string
	Bell             bool
	InputHeight      int
	AdaptiveHeight   bool
	HeightMaxPercent int
	Theme
}

func defaultTheme() Theme {
	return Theme{
		Background:             "#111318",
		TextColor:              "#E6E6E6",
		BorderColor:            "#444B5A",
		AccentColor:            "#7C5CFC",
		HoverTextColor:         "#F9F871",
		MutedTextColor:         "#707786",
		MentionBackgroundColor:   "#3C3622",
		SelectionBackgroundColor: "#3D5275",
	}
}

func defaultConfig() Config {
	return Config{
		Name:             "Anonymous",
		Bell:             true,
		InputHeight:      1,
		AdaptiveHeight:   false,
		HeightMaxPercent: 30,
		UserColor:        "#7CFFCB",
		Theme:            defaultTheme(),
	}
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}

	return filepath.Join(dir, "zealot", "config.toml")
}

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	path := configPath()

	data, err := os.ReadFile(path)

	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return cfg, err
		}

		if err := os.WriteFile(
			path,
			[]byte(defaultConfigFile()),
			0600,
		); err != nil {
			return cfg, err
		}

		if err := ensureThemeLibrary(); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	if err != nil {
		return cfg, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"`)

		switch key {
		case "name":
			cfg.Name = value
		case "user_color":
			cfg.UserColor = value
		case "background":
			cfg.Theme.Background = value
		case "text_color":
			cfg.Theme.TextColor = value
		case "border_color":
			cfg.Theme.BorderColor = value
		case "accent_color":
			cfg.Theme.AccentColor = value
		case "hover_text_color":
			cfg.Theme.HoverTextColor = value
		case "muted_text_color":
			cfg.Theme.MutedTextColor = value
		case "mention_background_color":
			cfg.Theme.MentionBackgroundColor = value
		case "selection_background_color":
			cfg.Theme.SelectionBackgroundColor = value
		case "bell":
			if enabled, err := strconv.ParseBool(value); err == nil {
				cfg.Bell = enabled
			}
		case "input_height":
			if height, err := strconv.Atoi(value); err == nil {
				cfg.InputHeight = height
			}
		case "adaptive_height", "adaptive_input": // Keep accepting the earlier key as an alias.
			if enabled, err := strconv.ParseBool(value); err == nil {
				cfg.AdaptiveHeight = enabled
			}
		case "height_max_percent", "input_max_percent": // Keep accepting the earlier key as an alias.
			if percent, err := strconv.Atoi(value); err == nil {
				cfg.HeightMaxPercent = percent
			}
		}
	}

	if cfg.InputHeight < 1 {
		cfg.InputHeight = 1
	}
	if cfg.InputHeight > 8 {
		cfg.InputHeight = 8
	}
	if cfg.HeightMaxPercent < 1 {
		cfg.HeightMaxPercent = 1
	}
	if cfg.HeightMaxPercent > 100 {
		cfg.HeightMaxPercent = 100
	}

	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "Anonymous"
	}

	if err := ensureThemeLibrary(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func defaultConfigFile() string {
	return `# Zealot configuration
#
# Colors may be ANSI colors ("12"), 256-color numbers,
# or hexadecimal colors ("#7CFFCB").

name = "Anonymous"

user_color = "#7CFFCB"

bell = true

# Minimum visible text rows in the composer (1-8).
input_height = 1

# Grow the composer as you type (true/false).
adaptive_height = false

# Maximum share of terminal height used by the composer (1-100).
height_max_percent = 30

# Main theme palette.
background = "#111318"
text_color = "#E6E6E6"
border_color = "#444B5A"
accent_color = "#7C5CFC"
hover_text_color = "#F9F871"
muted_text_color = "#707786"
mention_background_color = "#3C3622"
selection_background_color = "#3D5275"
`
}

func saveConfig(cfg Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	content := fmt.Sprintf(`# Zealot configuration
# Colors may be ANSI colors ("12"), 256-color numbers, or hexadecimal colors ("#7CFFCB").

name = %q
user_color = %q
bell = %t
input_height = %d
adaptive_height = %t
height_max_percent = %d
# Main theme palette.
background = %q
text_color = %q
border_color = %q
accent_color = %q
hover_text_color = %q
muted_text_color = %q
mention_background_color = %q
selection_background_color = %q
`, cfg.Name, cfg.UserColor, cfg.Bell, cfg.InputHeight, cfg.AdaptiveHeight, cfg.HeightMaxPercent, cfg.Theme.Background, cfg.Theme.TextColor, cfg.Theme.BorderColor, cfg.Theme.AccentColor,
		cfg.Theme.HoverTextColor, cfg.Theme.MutedTextColor, cfg.Theme.MentionBackgroundColor, cfg.Theme.SelectionBackgroundColor)
	return os.WriteFile(path, []byte(content), 0600)
}

func validUserColor(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		if len(value) != 7 {
			return false
		}
		_, err := strconv.ParseUint(value[1:], 16, 24)
		return err == nil
	}
	n, err := strconv.Atoi(value)
	return err == nil && n >= 0 && n <= 255
}
