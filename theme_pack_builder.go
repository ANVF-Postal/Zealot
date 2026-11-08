package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
)

// packFileOps lets tests simulate write/rename failures without needing a full
// disk or privileged filesystem. Production operations use the OS directly.
type packFileOps struct {
	write   func(string, []byte) error
	rename  func(string, string) error
	restore func(string, string) error
}

func defaultPackFileOps() packFileOps {
	return packFileOps{
		write: func(path string, data []byte) error {
			return os.WriteFile(path, data, 0600)
		},
		rename:  os.Rename,
		restore: os.Rename,
	}
}

// createPackedThemeFile first writes every output in a private staging
// directory. Only after all writes succeed does it archive original files,
// install remaining live themes, and publish the new pack last.
// In-progress failures restore originals from their untouched archives.
func createPackedThemeFile(name string, selected []themeListEntry) (string, string, error) {
	return createPackedThemeFileWithOps(name, selected, defaultPackFileOps())
}

func createPackedThemeFileWithOps(name string, selected []themeListEntry, ops packFileOps) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", fmt.Errorf("pack name cannot be empty")
	}
	if len(selected) == 0 {
		return "", "", fmt.Errorf("select at least one custom theme")
	}
	filename := "pack-" + strings.TrimPrefix(themeFilenameStem(name), "theme-") + ".toml"
	newPath := filepath.Join(themesDir(), filename)
	if _, err := os.Stat(newPath); err == nil {
		return "", "", fmt.Errorf("pack %q already exists; choose another name", filename)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("check pack filename: %w", err)
	}

	type source struct {
		filename  string
		original  []byte
		remaining []ThemePreset
		archive   string
		staged    string
		archived  bool
		installed bool
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
			src = &source{
				filename: entry.FileName,
				original: data,
				remaining: append([]ThemePreset(nil), pack.Themes...),
			}
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

	// Create the staging directory on the same filesystem, so the final
	// operations can use renames rather than writing partially completed TOML.
	if err := os.MkdirAll(themesDir(), 0700); err != nil {
		return "", "", fmt.Errorf("create themes directory: %w", err)
	}
	staging, err := os.MkdirTemp(themesDir(), ".pack-staging-")
	if err != nil {
		return "", "", fmt.Errorf("prepare theme pack: %w", err)
	}
	defer os.RemoveAll(staging)

	packBytes, err := encodeThemePack(pack)
	if err != nil {
		return "", "", err
	}
	stagedPack := filepath.Join(staging, filename)
	if err := ops.write(stagedPack, packBytes); err != nil {
		return "", "", fmt.Errorf("prepare new theme pack: %w", err)
	}
	for _, src := range sources {
		if len(src.remaining) == 0 {
			continue
		}
		var existing ThemePack
		if _, err := toml.Decode(string(src.original), &existing); err != nil {
			return "", "", fmt.Errorf("prepare remaining themes: %w", err)
		}
		existing.Themes = src.remaining
		content, err := encodeThemePack(existing)
		if err != nil {
			return "", "", fmt.Errorf("prepare remaining themes: %w", err)
		}
		src.staged = filepath.Join(staging, src.filename)
		if err := ops.write(src.staged, content); err != nil {
			return "", "", fmt.Errorf("prepare remaining themes: %w", err)
		}
	}

	archiveDir := filepath.Join(themesDir(), "packed")
	if err := os.MkdirAll(archiveDir, 0700); err != nil {
		return "", "", fmt.Errorf("prepare archive directory: %w", err)
	}
	// Find unique names before any original source is moved.
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
				return "", "", fmt.Errorf("check archive file: %w", err)
			}
		}
		if src.archive == "" {
			return "", "", fmt.Errorf("cannot archive theme file %q", src.filename)
		}
	}

	// Any failure after the first archive move must restore every source.
	// Never discard an archived original if restoration fails; report its
	// location so it can be recovered manually.
	rollback := func(cause error) (string, string, error) {
		var recoveryErrors []error
		for i := len(sources) - 1; i >= 0; i-- {
			src := sources[i]
			if !src.archived {
				continue
			}
			live := filepath.Join(themesDir(), src.filename)
			archived := filepath.Join(archiveDir, src.archive)
			if src.installed {
				if err := os.Remove(live); err != nil {
					recoveryErrors = append(recoveryErrors, fmt.Errorf("remove incomplete %q: %w", src.filename, err))
					continue
				}
			}
			if err := ops.restore(archived, live); err != nil {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("restore %q from packed/%s: %w", src.filename, src.archive, err))
			}
		}
		if len(recoveryErrors) != 0 {
			return "", "", fmt.Errorf("%w; automatic recovery was incomplete. Original files are retained in themes/packed/; restore them before trying again", errors.Join(cause, errors.Join(recoveryErrors...)))
		}
		return "", "", cause
	}

	// Commit phase. All TOML has been written before any original moves.
	for _, src := range sources {
		if err := ops.rename(filepath.Join(themesDir(), src.filename), filepath.Join(archiveDir, src.archive)); err != nil {
			return rollback(fmt.Errorf("archive %q: %w", src.filename, err))
		}
		src.archived = true
		if src.staged != "" {
			if err := ops.rename(src.staged, filepath.Join(themesDir(), src.filename)); err != nil {
				return rollback(fmt.Errorf("install remaining themes in %q: %w", src.filename, err))
			}
			src.installed = true
		}
	}
	// Publishing the new pack is deliberately last: a failed transaction
	// never leaves a visible duplicate pack in the theme library.
	if err := ops.rename(stagedPack, newPath); err != nil {
		return rollback(fmt.Errorf("publish new theme pack: %w", err))
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
	m.themeDetailsScroll = 0
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
	m.themeDetailsScroll = 0
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
	m.themeDetailsScroll = 0
	m.themeEditError = ""
}

// Keep filesystem details available, but make the failure recognizable in the
// pack window rather than leaving a raw system error as the only explanation.
func packCreationError(err error) string {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return "Couldn't complete packing (disk full). " + err.Error()
	case errors.Is(err, os.ErrPermission):
		return "Couldn't complete packing (permissions). " + err.Error()
	default:
		return "Couldn't complete packing. " + err.Error()
	}
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
		m.themeEditError = packCreationError(err)
		m.themeDetailsScroll = m.maxThemeDetailsScroll()
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
