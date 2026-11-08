package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// createPackedThemeFile moves selected custom presets into a new pack, while
// archiving their original files in themes/packed. Multi-theme source packs
// remain visible with just their unselected presets.
func createPackedThemeFile(name string, selected []themeListEntry) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", fmt.Errorf("pack name cannot be empty")
	}
	if len(selected) == 0 {
		return "", "", fmt.Errorf("select at least one custom theme")
	}
	filename := "pack-" + strings.TrimPrefix(themeFilenameStem(name), "theme-") + ".toml"
	if strings.EqualFold(filename, defaultThemePackFilename) {
		return "", "", fmt.Errorf("reserved pack filename")
	}
	newPath := filepath.Join(themesDir(), filename)
	if _, err := os.Stat(newPath); err == nil {
		return "", "", fmt.Errorf("pack %q already exists; choose another name", filename)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("check pack filename: %w", err)
	}

	type source struct {
		filename string
		original []byte
		remaining []ThemePreset
		archive string
	}
	sources := make([]*source, 0)
	byFile := make(map[string]*source)
	presets := make([]ThemePreset, 0, len(selected))
	ids := make(map[string]bool)
	seen := make(map[string]bool)
	for _, entry := range selected {
		if !themeEntryEditable(entry) || filepath.Base(entry.FileName) != entry.FileName {
			return "", "", fmt.Errorf("built-in and invalid themes cannot be packed")
		}
		key := strings.ToLower(entry.FileName + "/" + entry.Preset.ID)
		if seen[key] {
			return "", "", fmt.Errorf("theme %q was selected more than once", entry.Preset.Name)
		}
		seen[key] = true
		src := byFile[entry.FileName]
		if src == nil {
			data, err := os.ReadFile(filepath.Join(themesDir(), entry.FileName))
			if err != nil {
				return "", "", fmt.Errorf("read source theme %q: %w", entry.FileName, err)
			}
			var pack ThemePack
			if _, err := toml.Decode(string(data), &pack); err != nil {
				return "", "", err
			}
			if err := validateThemePack(pack); err != nil {
				return "", "", err
			}
			src = &source{filename: entry.FileName, original: data, remaining: append([]ThemePreset(nil), pack.Themes...)}
			byFile[entry.FileName] = src
			sources = append(sources, src)
		}
		found := -1
		for i, preset := range src.remaining {
			if strings.EqualFold(preset.ID, entry.Preset.ID) && preset == entry.Preset {
				found = i
				break
			}
		}
		if found < 0 {
			return "", "", fmt.Errorf("theme %q changed on disk; reopen the library", entry.Preset.Name)
		}
		src.remaining = append(src.remaining[:found], src.remaining[found+1:]...)
		preset := entry.Preset
		base := preset.ID
		for suffix := 2; ids[strings.ToLower(preset.ID)]; suffix++ {
			preset.ID = fmt.Sprintf("%s-%d", base, suffix)
		}
		ids[strings.ToLower(preset.ID)] = true
		presets = append(presets, preset)
	}
	pack := ThemePack{SchemaVersion: themePackSchemaVersion, PackName: name, Themes: presets}
	if err := validateThemePack(pack); err != nil {
		return "", "", err
	}
	archiveDir := filepath.Join(themesDir(), "packed")
	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return "", "", fmt.Errorf("create archive directory: %w", err)
	}
	// Reserve distinct archive names before moving any files.
	reserved := make(map[string]bool)
	for _, src := range sources {
		base := strings.TrimSuffix(src.filename, filepath.Ext(src.filename))
		for n := 1; n <= 10000; n++ {
			candidate := src.filename
			if n > 1 {
				candidate = fmt.Sprintf("%s-%d.toml", base, n)
			}
			if reserved[strings.ToLower(candidate)] {
				continue
			}
			if _, err := os.Stat(filepath.Join(archiveDir, candidate)); errors.Is(err, os.ErrNotExist) {
				src.archive = candidate
				reserved[strings.ToLower(candidate)] = true
				break
			} else if err != nil {
				return "", "", err
			}
		}
		if src.archive == "" {
			return "", "", fmt.Errorf("cannot archive theme file %q", src.filename)
		}
	}
	if err := saveThemePack(filename, pack); err != nil {
		return "", "", err
	}
	// On failure, restore each original source and remove the new pack.
	completed := make([]*source, 0, len(sources))
	rollback := func(err error) (string, string, error) {
		for i := len(completed)-1; i >= 0; i-- {
			src := completed[i]
			live := filepath.Join(themesDir(), src.filename)
			archived := filepath.Join(archiveDir, src.archive)
			_ = os.Remove(live)
			_ = os.Rename(archived, live)
		}
		_ = os.Remove(newPath)
		return "", "", err
	}
	for _, src := range sources {
		live := filepath.Join(themesDir(), src.filename)
		archived := filepath.Join(archiveDir, src.archive)
		if err := os.Rename(live, archived); err != nil {
			return rollback(fmt.Errorf("archive %q: %w", src.filename, err))
		}
		completed = append(completed, src)
		if len(src.remaining) != 0 {
			var old ThemePack
			if _, err := toml.Decode(string(src.original), &old); err != nil {
				return rollback(err)
			}
			old.Themes = src.remaining
			if err := saveThemePack(src.filename, old); err != nil {
				return rollback(fmt.Errorf("restore unselected themes from %q: %w", src.filename, err))
			}
		}
	}
	return filename, presets[0].ID, nil
}

func themePackSelectionKey(entry themeListEntry) string {
	return strings.ToLower(entry.FileName + "/" + entry.Preset.ID)
}

func (m *model) beginThemePacking() {
	entry, ok := m.selectedThemeEntry()
	if !ok || !themeEntryEditable(entry) || m.themeDirty || m.themeEditField != "" {
		return
	}
	m.themePacking = true
	m.themePackNaming = false
	m.themePackSelected = map[string]bool{themePackSelectionKey(entry): true}
	m.themeEditError = ""
}

func (m *model) toggleThemePacking(index int) {
	if !m.themePacking || m.themePackNaming || index < 0 || index >= len(m.themeEntries) {
		return
	}
	entry := m.themeEntries[index]
	if !themeEntryEditable(entry) {
		return
	}
	key := themePackSelectionKey(entry)
	if m.themePackSelected[key] {
		delete(m.themePackSelected, key)
	} else {
		m.themePackSelected[key] = true
	}
	m.themeEditError = ""
}

func (m *model) beginThemePackNaming() {
	if !m.themePacking || m.themePackNaming {
		return
	}
	if len(m.themePackSelected) == 0 {
		m.themeEditError = "Select at least one custom theme to pack."
		return
	}
	m.themePackNaming = true
	m.themeComposerBackup = m.input.Value()
	m.input.SetValue("")
	m.input.Placeholder = "Pack name · Enter to save · Esc to cancel"
	m.input.Focus()
	m.themeEditError = ""
}

func (m *model) cancelThemePackNaming() {
	m.input.SetValue(m.themeComposerBackup)
	m.themeComposerBackup = ""
	m.input.Placeholder = defaultComposerPlaceholder
	m.themePackNaming = false
	m.themeEditError = ""
}

func (m *model) finishThemePacking() {
	if !m.themePacking || !m.themePackNaming {
		return
	}
	selected := make([]themeListEntry, 0, len(m.themePackSelected))
	for _, entry := range m.themeEntries {
		if m.themePackSelected[themePackSelectionKey(entry)] {
			selected = append(selected, entry)
		}
	}
	filename, id, err := createPackedThemeFile(m.input.Value(), selected)
	if err != nil {
		m.themeEditError = err.Error()
		return
	}
	m.cancelThemePackNaming()
	m.themePacking = false
	m.themePackSelected = nil
	m.reloadThemeEntries(filename, id)
}

func (m *model) cancelThemePacking() {
	if m.themePackNaming {
		m.cancelThemePackNaming()
		return
	}
	m.themePacking = false
	m.themePackSelected = nil
	m.themeEditError = ""
}
