package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description,omitempty"`
	Colors      Theme  `toml:"colors"`
}

func defaultThemePack() ThemePack {
	return ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName:      "Zealot defaults",
		Themes: []ThemePreset{
			{
				ID:          "zealot",
				Name:        "Zealot",
				Description: "A balanced dark terminal palette with clear contrast and a restrained violet accent.",
				Colors:      defaultTheme(),
			},
			{
				ID:          "autumn",
				Name:        "Autumn",
				Description: "A warm late-autumn palette with deep orange-brown surfaces and bright orange, amber, and rust accents.",
				Colors: Theme{
					Background:               "#2D170A",
					TextColor:                "#F7E8D8",
					BorderColor:              "#7A442A",
					AccentColor:              "#FF8C2A",
					HoverTextColor:           "#FFD166",
					MutedTextColor:           "#C18B62",
					MentionBackgroundColor:   "#622F1C",
					SelectionBackgroundColor: "#92451F",
				},
			},
			{
				ID:          "limetime",
				Name:        "Limetime",
				Description: "Heavy and high contrast. Based around the accent color #AEFF00",
				Colors: Theme{
					Background:               "#0E1009",
					TextColor:                "#E6E6E6",
					BorderColor:              "#2A3416",
					AccentColor:              "#AEFF00",
					HoverTextColor:           "#F9F871",
					MutedTextColor:           "#869763",
					MentionBackgroundColor:   "#191D0F",
					SelectionBackgroundColor: "#007000",
				},
			},
			{
				ID:          "green-phosphor",
				Name:        "Green Phosphor",
				Description: "Classic monochrome CRT terminal glow, with every interface color rendered in shades of phosphor green.",
				Colors: Theme{
					Background:               "#030703",
					TextColor:                "#79DC4B",
					BorderColor:              "#173C1A",
					AccentColor:              "#69F02E",
					HoverTextColor:           "#A0F569",
					MutedTextColor:           "#3F8041",
					MentionBackgroundColor:   "#0A230D",
					SelectionBackgroundColor: "#1C5B22",
				},
			},
			{
				ID:          "fbi",
				Name:        "FBI",
				Description: "Deep federal blue, seal-inspired gold, and ceremonial red and green accents.",
				Colors: Theme{
					Background:               "#07366B",
					TextColor:                "#F5F3E9",
					BorderColor:              "#AC9150",
					AccentColor:              "#FFD348",
					HoverTextColor:           "#FF8580",
					MutedTextColor:           "#B5C9DF",
					MentionBackgroundColor:   "#723B47",
					SelectionBackgroundColor: "#27654C",
				},
			},
			{
				ID:          "putty-classic",
				Name:        "PuTTY Classic",
				Description: "Classic PuTTY ANSI terminal colors on black, with bright yellow, blue and green plus cyan, magenta and red.",
				Colors: Theme{
					Background:               "#000000",
					TextColor:                "#BBBBBB",
					BorderColor:              "#5555FF",
					AccentColor:              "#FFFF55",
					HoverTextColor:           "#55FF55",
					MutedTextColor:           "#00BBBB",
					MentionBackgroundColor:   "#BB00BB",
					SelectionBackgroundColor: "#BB0000",
				},
			},
			{
				ID:          "darkroom",
				Name:        "Darkroom",
				Description: "Dim red-on-red tones inspired by photographic darkrooms under a safelight.",
				Colors: Theme{
					Background:               "#120506",
					TextColor:                "#D68A8A",
					BorderColor:              "#542024",
					AccentColor:              "#FF5555",
					HoverTextColor:           "#FF9A9A",
					MutedTextColor:           "#955054",
					MentionBackgroundColor:   "#2D0C10",
					SelectionBackgroundColor: "#71252A",
				},
			},
			{
				ID:          "blackout",
				Name:        "Blackout",
				Description: "Pure black with uncompromising red interface highlights, a high-contrast companion to Darkroom.",
				Colors: Theme{
					Background:               "#000000",
					TextColor:                "#DA6B6B",
					BorderColor:              "#481111",
					AccentColor:              "#FF3030",
					HoverTextColor:           "#FF7070",
					MutedTextColor:           "#8F3838",
					MentionBackgroundColor:   "#240000",
					SelectionBackgroundColor: "#690909",
				},
			},
			{
				ID:          "glados",
				Name:        "GladOS",
				Description: "Amber-orange phosphor on near-black, inspired by GLaDOS's retro computer terminal credits in Portal.",
				Colors: Theme{
					Background:               "#080502",
					TextColor:                "#E9A54D",
					BorderColor:              "#713F13",
					AccentColor:              "#FF9900",
					HoverTextColor:           "#FFD079",
					MutedTextColor:           "#945822",
					MentionBackgroundColor:   "#261305",
					SelectionBackgroundColor: "#713706",
				},
			},
			{
				ID:          "orchid",
				Name:        "Orchid",
				Description: "Electric orchid and hot pink neon against deep violet, with bright cyan highlights.",
				Colors: originalArcadeColors(),
			},
			{
				ID:          "arcade-neon",
				Name:        "Arcade",
				Description: "Vibrant 90's arcade colors complete with glowsticks!",
				Colors: Theme{
					Background:               "#090A18",
					TextColor:                "#F7F2DE",
					BorderColor:              "#28DBFF",
					AccentColor:              "#FFE600",
					HoverTextColor:           "#FF6B21",
					MutedTextColor:           "#AE7BFF",
					MentionBackgroundColor:   "#7E1D68",
					SelectionBackgroundColor: "#208D3C",
				},
			},
			{
				ID:          "noir",
				Name:        "Noir",
				Description: "Black-and-white cinema in terminal form: charcoal, silver, graphite, and stark monochrome highlights.",
				Colors: Theme{
					Background:               "#111111",
					TextColor:                "#D6D6D6",
					BorderColor:              "#505050",
					AccentColor:              "#F2F2F2",
					HoverTextColor:           "#FFFFFF",
					MutedTextColor:           "#858585",
					MentionBackgroundColor:   "#292929",
					SelectionBackgroundColor: "#626262",
				},
			},
			{
				ID:          "commodore",
				Name:        "Commodore",
				Description: "Warm vintage computer plastic with raised cream-beige borders and colorful retro hardware accents.",
				Colors: Theme{
					Background:               "#BCAA7C",
					TextColor:                "#3A3024",
					BorderColor:              "#E2D4AE",
					AccentColor:              "#205A9C",
					HoverTextColor:           "#D94B3D",
					MutedTextColor:           "#6F614A",
					MentionBackgroundColor:   "#A79670",
					SelectionBackgroundColor: "#5FA35B",
				},
			},
			{
				ID:          "woodland",
				Name:        "Woodland",
				Description: "An earthy theme inspired by military woodland camo. Over n' out.",
				Colors: Theme{
					Background:               "#273A2B",
					TextColor:                "#DCD6BA",
					BorderColor:              "#657850",
					AccentColor:              "#C3B48A",
					HoverTextColor:           "#E6D5A9",
					MutedTextColor:           "#98A88D",
					MentionBackgroundColor:   "#40583D",
					SelectionBackgroundColor: "#566A47",
				},
			},
			{
				ID:          "maximus",
				Name:        "Maximus",
				Description: "Zealot theme inspired by the UI colors of the multiband compressor Maximus",
				Colors: Theme{
					Background:               "#262F31",
					TextColor:                "#D7D8CB",
					BorderColor:              "#576267",
					AccentColor:              "#7DC053",
					HoverTextColor:           "#C0CB46",
					MutedTextColor:           "#89988B",
					MentionBackgroundColor:   "#314237",
					SelectionBackgroundColor: "#C94E46",
				},
			},
		},
	}
}

// originalArcadeColors is the original violet-and-pink Arcade palette, now
// named Orchid. It also lets migration identify an unmodified old preset.
func originalArcadeColors() Theme {
	return Theme{
		Background:               "#0B061A",
		TextColor:                "#E3DCFF",
		BorderColor:              "#8438D6",
		AccentColor:              "#FF39B8",
		HoverTextColor:           "#36F5FF",
		MutedTextColor:           "#9178B5",
		MentionBackgroundColor:   "#391152",
		SelectionBackgroundColor: "#287A39",
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
		return ensureMissingDefaultThemes(defaultThemePackPath())
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

// ensureMissingDefaultThemes adds newly shipped built-ins to an existing
// default pack without replacing its contents or any user-customized presets.
func ensureMissingDefaultThemes(path string) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read existing default theme pack: %w", err)
	}

	var pack ThemePack
	if _, err := toml.Decode(string(existing), &pack); err != nil {
		return fmt.Errorf("decode existing default theme pack: %w", err)
	}
	// A pre-existing file without a schema version is not a pack Zealot
	// can safely migrate. Preserve it exactly and leave config loading usable.
	// Versioned packs, including unsupported future versions, are validated
	// normally so we never append data using an unknown schema.
	if pack.SchemaVersion == 0 {
		return nil
	}
	if err := validateThemePack(pack); err != nil {
		return fmt.Errorf("validate existing default theme pack: %w", err)
	}

	// The first Arcade release was actually the violet/pink palette now
	// called Orchid. Rename only an untouched shipped preset, preserving
	// comments and all other user content byte-for-byte. A customized old
	// Arcade stays untouched; the new Orchid and Arcade can be added beside it.
	for i := range pack.Themes {
		preset := pack.Themes[i]
		if preset.ID != "arcade" || preset.Name != "Arcade" ||
			preset.Description != "Vibrant 90's arcade colors complete with glowsticks!" ||
			preset.Colors != originalArcadeColors() {
			continue
		}
		updated, ok := renameOriginalArcadePreset(string(existing))
		if !ok {
			break // Nonstandard formatting: leave the file unmodified.
		}
		if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
			return fmt.Errorf("rename original Arcade to Orchid: %w", err)
		}
		existing = []byte(updated)
		pack.Themes[i].ID = "orchid"
		pack.Themes[i].Name = "Orchid"
		pack.Themes[i].Description = "Electric orchid and hot pink neon against deep violet, with bright cyan highlights."
		break
	}

	known := make(map[string]struct{}, len(pack.Themes))
	for _, preset := range pack.Themes {
		known[strings.ToLower(strings.TrimSpace(preset.ID))] = struct{}{}
	}

	var missing []ThemePreset
	for _, preset := range defaultThemePack().Themes {
		key := strings.ToLower(strings.TrimSpace(preset.ID))
		if _, exists := known[key]; exists {
			continue
		}
		missing = append(missing, preset)
	}
	if len(missing) == 0 {
		return nil
	}

	// Encode only the missing array-table entries so existing comments,
	// formatting, and customized built-in palettes remain untouched.
	var additions bytes.Buffer
	appendDocument := struct {
		Themes []ThemePreset `toml:"themes"`
	}{Themes: missing}
	if err := toml.NewEncoder(&additions).Encode(appendDocument); err != nil {
		return fmt.Errorf("encode missing default themes: %w", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open existing default theme pack: %w", err)
	}
	separator := []byte("\n")
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = []byte("\n\n")
	}
	if _, err := file.Write(separator); err != nil {
		_ = file.Close()
		return fmt.Errorf("separate new default themes: %w", err)
	}
	if _, err := file.Write(additions.Bytes()); err != nil {
		_ = file.Close()
		return fmt.Errorf("append missing default themes: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close updated default theme pack: %w", err)
	}
	return nil
}

// renameOriginalArcadePreset changes only metadata within the matching TOML
// array table. This keeps existing colors, comments, and other presets intact.
func renameOriginalArcadePreset(content string) (string, bool) {
	const table = "[[themes]]"
	for pos := strings.Index(content, table); pos >= 0; {
		end := strings.Index(content[pos+len(table):], table)
		if end < 0 {
			end = len(content)
		} else {
			end += pos + len(table)
		}
		block := content[pos:end]
		if strings.Contains(block, "id = \"arcade\"") &&
			strings.Contains(block, "name = \"Arcade\"") &&
			strings.Contains(block, "description = \"Vibrant 90's arcade colors complete with glowsticks!\"") {
			block = strings.Replace(block, "id = \"arcade\"", "id = \"orchid\"", 1)
			block = strings.Replace(block, "name = \"Arcade\"", "name = \"Orchid\"", 1)
			block = strings.Replace(block,
				"description = \"Vibrant 90's arcade colors complete with glowsticks!\"",
				"description = \"Electric orchid and hot pink neon against deep violet, with bright cyan highlights.\"", 1)
			return content[:pos] + block + content[end:], true
		}
		if end == len(content) {
			break
		}
		pos = strings.Index(content[end:], table)
		if pos >= 0 {
			pos += end
		}
	}
	return content, false
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
	// Keep the built-in Zealot pack first, then sort user packs by filename.
	// os.ReadDir sorts names, but explicitly ordering the built-in pack first
	// makes initial active-theme selection stable even when user packs sort
	// before "default-themes.toml".
	sort.SliceStable(packs, func(i, j int) bool {
		const defaultFile = "default-themes.toml"
		iDefault := packs[i].FileName == defaultFile
		jDefault := packs[j].FileName == defaultFile
		if iDefault != jDefault {
			return iDefault
		}
		return strings.ToLower(packs[i].FileName) < strings.ToLower(packs[j].FileName)
	})
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

// themeFilenameStem turns a display name into a predictable, portable TOML filename.
func themeFilenameStem(name string) string {
	var slug strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			slug.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && slug.Len() > 0 {
				slug.WriteByte('-')
				lastDash = true
			}
		}
	}
	value := strings.Trim(slug.String(), "-")
	if value == "" {
		value = "custom"
	}
	return "theme-" + value
}

func nextThemeFilename(name string) (string, error) {
	stem := themeFilenameStem(name)
	for suffix := 1; suffix < 10000; suffix++ {
		candidate := stem + ".toml"
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d.toml", stem, suffix)
		}
		_, err := os.Stat(filepath.Join(themesDir(), candidate))
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check theme filename %q: %w", candidate, err)
		}
	}
	return "", fmt.Errorf("could not find an available filename for theme %q", name)
}

// createThemePresetFile stores one editable theme in its own TOML file.
// Each clone/new theme is independent of the shipped default pack.
func createThemePresetFile(preset ThemePreset) (themeListEntry, error) {
	if strings.TrimSpace(preset.Name) == "" {
		return themeListEntry{}, fmt.Errorf("theme name cannot be empty")
	}
	if err := validateThemeColors(preset.Colors); err != nil {
		return themeListEntry{}, err
	}
	if err := os.MkdirAll(themesDir(), 0700); err != nil {
		return themeListEntry{}, fmt.Errorf("create themes directory: %w", err)
	}
	filename, err := nextThemeFilename(preset.Name)
	if err != nil {
		return themeListEntry{}, err
	}
	id := strings.TrimSuffix(strings.TrimPrefix(filename, "theme-"), ".toml")
	preset.ID = id
	pack := ThemePack{
		SchemaVersion: themePackSchemaVersion,
		PackName:      "Custom themes",
		Themes:        []ThemePreset{preset},
	}
	if err := saveThemePack(filename, pack); err != nil {
		return themeListEntry{}, err
	}
	return themeListEntry{Preset: preset, PackName: pack.PackName, ThemeCount: len(pack.Themes), FileName: filename}, nil
}

// saveThemePresetFile updates only an editable preset. A one-theme file follows
// the display name when renamed; multi-theme packs keep their shared filename.
func saveThemePresetFile(entry themeListEntry, preset ThemePreset) (themeListEntry, error) {
	if entry.FileName == "" || strings.EqualFold(entry.FileName, defaultThemePackFilename) {
		return themeListEntry{}, fmt.Errorf("Zealot's built-in themes are read-only; clone one to edit it")
	}
	if err := validateThemeColors(preset.Colors); err != nil {
		return themeListEntry{}, err
	}
	if strings.TrimSpace(preset.Name) == "" {
		return themeListEntry{}, fmt.Errorf("theme name cannot be empty")
	}

	oldPath := filepath.Join(themesDir(), entry.FileName)
	var pack ThemePack
	if _, err := toml.DecodeFile(oldPath, &pack); err != nil {
		return themeListEntry{}, fmt.Errorf("read theme pack %q: %w", entry.FileName, err)
	}
	if err := validateThemePack(pack); err != nil {
		return themeListEntry{}, fmt.Errorf("validate theme pack %q: %w", entry.FileName, err)
	}
	found := false
	for i := range pack.Themes {
		if strings.EqualFold(pack.Themes[i].ID, entry.Preset.ID) {
			pack.Themes[i] = preset
			found = true
			break
		}
	}
	if !found {
		return themeListEntry{}, fmt.Errorf("theme %q no longer exists in %q", entry.Preset.ID, entry.FileName)
	}

	newFilename := entry.FileName
	if len(pack.Themes) == 1 && !strings.HasPrefix(strings.ToLower(entry.FileName), "pack-") {
		newFilename = themeFilenameStem(preset.Name) + ".toml"
	}
	newPath := filepath.Join(themesDir(), newFilename)
	if newFilename != entry.FileName {
		if _, err := os.Stat(newPath); err == nil {
			return themeListEntry{}, fmt.Errorf("a theme file named %q already exists; choose another name", newFilename)
		} else if !errors.Is(err, os.ErrNotExist) {
			return themeListEntry{}, fmt.Errorf("check destination theme file: %w", err)
		}
	}

	content, err := encodeThemePack(pack)
	if err != nil {
		return themeListEntry{}, err
	}
	if newFilename == entry.FileName {
		if err := os.WriteFile(oldPath, content, 0600); err != nil {
			return themeListEntry{}, fmt.Errorf("save theme file %q: %w", entry.FileName, err)
		}
	} else {
		file, err := os.OpenFile(newPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return themeListEntry{}, fmt.Errorf("create renamed theme file %q: %w", newFilename, err)
		}
		if _, err := file.Write(content); err != nil {
			_ = file.Close()
			_ = os.Remove(newPath)
			return themeListEntry{}, fmt.Errorf("write renamed theme file %q: %w", newFilename, err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(newPath)
			return themeListEntry{}, fmt.Errorf("close renamed theme file %q: %w", newFilename, err)
		}
		if err := os.Remove(oldPath); err != nil {
			_ = os.Remove(newPath)
			return themeListEntry{}, fmt.Errorf("could not finish renaming theme file %q: %w", entry.FileName, err)
		}
	}
	return themeListEntry{Preset: preset, PackName: pack.PackName, ThemeCount: len(pack.Themes), FileName: newFilename}, nil
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
	if err := saveThemeConfig(updated.Theme); err != nil {
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
