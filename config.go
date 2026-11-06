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
		if comment := tomlCommentOffset(line); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}

		if line == "" {
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

	if err := ensureThemeColorsInConfig(path, data, cfg.Theme); err != nil {
		return cfg, err
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

// configFileValues returns every setting managed by Config in stable file order.
// Legacy aliases are updated in place when present, rather than adding duplicate
// modern keys to an otherwise user-maintained config file.
func themeConfigFileValues(theme Theme) []configFileEntry {
	return []configFileEntry{
		{key: "background", value: strconv.Quote(theme.Background)},
		{key: "text_color", value: strconv.Quote(theme.TextColor)},
		{key: "border_color", value: strconv.Quote(theme.BorderColor)},
		{key: "accent_color", value: strconv.Quote(theme.AccentColor)},
		{key: "hover_text_color", value: strconv.Quote(theme.HoverTextColor)},
		{key: "muted_text_color", value: strconv.Quote(theme.MutedTextColor)},
		{key: "mention_background_color", value: strconv.Quote(theme.MentionBackgroundColor)},
		{key: "selection_background_color", value: strconv.Quote(theme.SelectionBackgroundColor)},
	}
}

func configFileValues(cfg Config) []configFileEntry {
	entries := []configFileEntry{
		{key: "name", value: strconv.Quote(cfg.Name)},
		{key: "user_color", value: strconv.Quote(cfg.UserColor)},
		{key: "bell", value: strconv.FormatBool(cfg.Bell)},
		{key: "input_height", value: strconv.Itoa(cfg.InputHeight)},
		{key: "adaptive_height", value: strconv.FormatBool(cfg.AdaptiveHeight)},
		{key: "height_max_percent", value: strconv.Itoa(cfg.HeightMaxPercent)},
	}
	return append(entries, themeConfigFileValues(cfg.Theme)...)
}

type configFileEntry struct {
	key   string
	value string
}

// configKeyCanonical maps older setting names onto their current Config field.
func configKeyCanonical(key string) string {
	switch key {
	case "adaptive_input":
		return "adaptive_height"
	case "input_max_percent":
		return "height_max_percent"
	default:
		return key
	}
}

// tomlCommentOffset finds a TOML comment marker outside a quoted string.
func tomlCommentOffset(value string) int {
	var quote byte
	escaped := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote == '"' && escaped {
			escaped = false
			continue
		}
		if quote == '"' && ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '#' {
			return i
		}
	}
	return -1
}

// replaceConfigValue changes only the value portion of one assignment. Its
// indentation, spacing around '=', trailing whitespace, and inline comment
// remain byte-for-byte intact.
func replaceConfigValue(line string, equals int, value string) string {
	rhs := line[equals+1:]
	comment := ""
	if offset := tomlCommentOffset(rhs); offset >= 0 {
		comment = rhs[offset:]
		rhs = rhs[:offset]
	}
	leading := rhs[:len(rhs)-len(strings.TrimLeft(rhs, " \t"))]
	trimmedRight := strings.TrimRight(rhs, " \t")
	trailing := rhs[len(trimmedRight):]
	return line[:equals+1] + leading + value + trailing + comment
}

// updateConfigValues refreshes only the supplied settings while retaining the
// user's layout and all comments/unknown keys. It also fills in settings absent
// from older configs, using the same defaults already present in Config.
func updateConfigValues(contents string, entries []configFileEntry) string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		values[entry.key] = entry.value
	}

	present := make(map[string]bool, len(entries))
	var output strings.Builder
	for _, rawLine := range strings.SplitAfter(contents, "\n") {
		if rawLine == "" {
			continue
		}
		ending := ""
		line := rawLine
		if strings.HasSuffix(line, "\n") {
			line = strings.TrimSuffix(line, "\n")
			ending = "\n"
		}
		if strings.HasSuffix(line, "\r") {
			line = strings.TrimSuffix(line, "\r")
			ending = "\r" + ending
		}

		equals := strings.IndexByte(line, '=')
		if equals >= 0 {
			key := strings.TrimSpace(line[:equals])
			canonical := configKeyCanonical(key)
			if value, managed := values[canonical]; managed {
				line = replaceConfigValue(line, equals, value)
				present[canonical] = true
			}
		}
		output.WriteString(line)
		output.WriteString(ending)
	}

	var missing []configFileEntry
	for _, entry := range entries {
		if !present[entry.key] {
			missing = append(missing, entry)
		}
	}
	if len(missing) > 0 {
		current := output.String()
		if current != "" && !strings.HasSuffix(current, "\n") {
			output.WriteByte('\n')
		}
		if current != "" {
			output.WriteByte('\n')
		}
		output.WriteString("# Settings added by Zealot.\n")
		for _, entry := range missing {
			output.WriteString(entry.key)
			output.WriteString(" = ")
			output.WriteString(entry.value)
			output.WriteByte('\n')
		}
	}
	return output.String()
}

// saveConfig updates managed values in place instead of rebuilding the file.
// This keeps comments, custom settings, and formatting intact when commands
// such as /rename and /color persist a change.
func saveConfigValues(entries []configFileEntry) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		contents = []byte(defaultConfigFile())
	}
	updated := updateConfigValues(string(contents), entries)
	return os.WriteFile(path, []byte(updated), 0600)
}

// saveConfig retains the full-config helper for config migration and tests.
// Runtime actions should use the narrower helpers below to avoid touching
// unrelated values at all.
func saveConfig(cfg Config) error {
	return saveConfigValues(configFileValues(cfg))
}

func saveConfigName(name string) error {
	return saveConfigValues([]configFileEntry{{key: "name", value: strconv.Quote(name)}})
}

func saveConfigUserColor(color string) error {
	return saveConfigValues([]configFileEntry{{key: "user_color", value: strconv.Quote(color)}})
}

func saveThemeConfig(theme Theme) error {
	return saveConfigValues(themeConfigFileValues(theme))
}

func normalizeUserColor(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "#") {
		if len(value) != 7 {
			return "", false
		}
		if _, err := strconv.ParseUint(value[1:], 16, 24); err != nil {
			return "", false
		}
		return strings.ToUpper(value), true
	}
	if len(value) == 6 {
		if _, err := strconv.ParseUint(value, 16, 24); err == nil {
			return "#" + strings.ToUpper(value), true
		}
	}
	n, err := strconv.Atoi(value)
	if err == nil && n >= 0 && n <= 255 {
		return value, true
	}
	return "", false
}

func validUserColor(value string) bool {
	// Stored themes still use canonical #RRGGBB strings or ANSI indices.
	// The editor separately accepts bare six-digit hex and normalizes it.
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


// ensureThemeColorsInConfig appends newly introduced palette keys that are
// absent from an older config. It preserves existing values, comments, and
// unrelated settings instead of rewriting the user's file.
func ensureThemeColorsInConfig(path string, contents []byte, theme Theme) error {
	present := make(map[string]bool)
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			present[strings.TrimSpace(parts[0])] = true
		}
	}

	entries := []struct {
		key   string
		value string
	}{
		{key: "background", value: theme.Background},
		{key: "text_color", value: theme.TextColor},
		{key: "border_color", value: theme.BorderColor},
		{key: "accent_color", value: theme.AccentColor},
		{key: "hover_text_color", value: theme.HoverTextColor},
		{key: "muted_text_color", value: theme.MutedTextColor},
		{key: "mention_background_color", value: theme.MentionBackgroundColor},
		{key: "selection_background_color", value: theme.SelectionBackgroundColor},
	}

	var missing []string
	for _, entry := range entries {
		if !present[entry.key] {
			missing = append(missing, fmt.Sprintf("%s = %q", entry.key, entry.value))
		}
	}
	if len(missing) == 0 {
		return nil
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open config to add missing theme colors: %w", err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, "\n# Theme colors added by Zealot."); err != nil {
		return fmt.Errorf("write theme config migration comment: %w", err)
	}
	for _, line := range missing {
		if _, err := fmt.Fprintln(file, line); err != nil {
			return fmt.Errorf("write missing theme config key: %w", err)
		}
	}
	return nil
}
