package main

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultComposerPlaceholder = "Type a message…"

func themeEntryEditable(entry themeListEntry) bool {
	return entry.FileName != "" && !strings.EqualFold(entry.FileName, defaultThemePackFilename)
}

func (m model) selectedThemeEntry() (themeListEntry, bool) {
	if len(m.themeEntries) == 0 || m.themeSelected < 0 || m.themeSelected >= len(m.themeEntries) {
		return themeListEntry{}, false
	}
	return m.themeEntries[m.themeSelected], true
}

func uniqueThemeName(base string, entries []themeListEntry) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "New Theme"
	}
	used := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		used[strings.ToLower(strings.TrimSpace(entry.Preset.Name))] = struct{}{}
	}
	candidate := base
	for suffix := 2; ; suffix++ {
		if _, exists := used[strings.ToLower(candidate)]; !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s %d", base, suffix)
	}
}

func (m *model) selectThemeEntry(index int) {
	if index < 0 || index >= len(m.themeEntries) {
		return
	}
	if m.themeDirty || m.themeEditField != "" {
		m.themeEditError = "Save the current edits before selecting another theme."
		return
	}
	m.themeSelected = index
	m.themeSavedPreset = m.themeEntries[index].Preset
	m.themeDirty = false
	m.themeEditError = ""
	m.clampThemeSelection()
}

func (m *model) beginThemeFieldEdit(field string) {
	entry, ok := m.selectedThemeEntry()
	if !ok || !themeEntryEditable(entry) || m.themeEditField != "" {
		return
	}
	value := themePresetFieldValue(entry.Preset, field)
	if value == "" {
		return
	}
	m.themeComposerBackup = m.input.Value()
	m.themeEditField = field
	m.themeEditError = ""
	m.input.Placeholder = "Edit " + themeFieldLabel(field) + " · Enter to confirm"
	m.input.SetValue(value)
	m.input.Focus()
}

func themeFieldLabel(field string) string {
	switch field {
	case "name":
		return "theme name"
	case "background":
		return "background color"
	case "text_color":
		return "main text color"
	case "border_color":
		return "border color"
	case "accent_color":
		return "accent color"
	case "hover_text_color":
		return "hover text color"
	case "muted_text_color":
		return "muted text color"
	case "mention_background_color":
		return "mention highlight color"
	case "selection_background_color":
		return "composer selection color"
	default:
		return "value"
	}
}

func themePresetFieldValue(preset ThemePreset, field string) string {
	switch field {
	case "name":
		return preset.Name
	case "background":
		return preset.Colors.Background
	case "text_color":
		return preset.Colors.TextColor
	case "border_color":
		return preset.Colors.BorderColor
	case "accent_color":
		return preset.Colors.AccentColor
	case "hover_text_color":
		return preset.Colors.HoverTextColor
	case "muted_text_color":
		return preset.Colors.MutedTextColor
	case "mention_background_color":
		return preset.Colors.MentionBackgroundColor
	case "selection_background_color":
		return preset.Colors.SelectionBackgroundColor
	default:
		return ""
	}
}

func setThemePresetField(preset *ThemePreset, field, value string) {
	if preset == nil {
		return
	}
	switch field {
	case "name":
		preset.Name = value
	case "background":
		preset.Colors.Background = value
	case "text_color":
		preset.Colors.TextColor = value
	case "border_color":
		preset.Colors.BorderColor = value
	case "accent_color":
		preset.Colors.AccentColor = value
	case "hover_text_color":
		preset.Colors.HoverTextColor = value
	case "muted_text_color":
		preset.Colors.MutedTextColor = value
	case "mention_background_color":
		preset.Colors.MentionBackgroundColor = value
	case "selection_background_color":
		preset.Colors.SelectionBackgroundColor = value
	}
}

func (m *model) restoreThemeComposer() {
	m.input.SetValue(m.themeComposerBackup)
	m.input.Placeholder = defaultComposerPlaceholder
	m.themeComposerBackup = ""
	m.themeEditField = ""
}

func (m *model) cancelThemeFieldEdit() {
	m.restoreThemeComposer()
	m.themeEditError = ""
}

func (m *model) finishThemeFieldEdit() {
	entry, ok := m.selectedThemeEntry()
	if !ok || m.themeEditField == "" {
		return
	}
	value := strings.TrimSpace(m.input.Value())
	if m.themeEditField == "name" {
		if value == "" {
			m.themeEditError = "Theme name cannot be empty."
			return
		}
		for index, other := range m.themeEntries {
			if index != m.themeSelected && strings.EqualFold(strings.TrimSpace(other.Preset.Name), value) {
				m.themeEditError = "A theme with that name already exists."
				return
			}
		}
	} else {
		if !validUserColor(value) {
			m.themeEditError = "Use a hex color such as #7C5CFC or an ANSI color number (0–255)."
			return
		}
		if strings.HasPrefix(value, "#") {
			value = strings.ToUpper(value)
		}
	}

	setThemePresetField(&entry.Preset, m.themeEditField, value)
	m.themeEntries[m.themeSelected] = entry
	m.themeDirty = entry.Preset != m.themeSavedPreset
	m.restoreThemeComposer()
	m.themeEditError = ""
}

func (m *model) reloadThemeEntries(selectFile, selectID string) {
	packs, err := loadThemePacks()
	if err != nil {
		m.themeEditError = err.Error()
		return
	}
	m.themeEntries = flattenThemePacks(packs)
	m.themeSelected = 0
	for i, entry := range m.themeEntries {
		if entry.FileName == selectFile && entry.Preset.ID == selectID {
			m.themeSelected = i
			break
		}
	}
	if len(m.themeEntries) > 0 {
		m.themeSavedPreset = m.themeEntries[m.themeSelected].Preset
	} else {
		m.themeSavedPreset = ThemePreset{}
	}
	m.themeDirty = false
	m.themeEditError = ""
	m.clampThemeSelection()
}

func (m *model) createFreshTheme() {
	if m.themeDirty || m.themeEditField != "" {
		m.themeEditError = "Save the current edits before creating another theme."
		return
	}
	name := uniqueThemeName("New Theme", m.themeEntries)
	entry, err := createThemePresetFile(ThemePreset{Name: name, Colors: defaultTheme()})
	if err != nil {
		m.themeEditError = err.Error()
		return
	}
	m.reloadThemeEntries(entry.FileName, entry.Preset.ID)
}

func (m *model) cloneSelectedTheme() {
	if m.themeDirty || m.themeEditField != "" {
		m.themeEditError = "Save the current edits before cloning this theme."
		return
	}
	current, ok := m.selectedThemeEntry()
	if !ok {
		return
	}
	name := uniqueThemeName(current.Preset.Name+" Copy", m.themeEntries)
	entry, err := createThemePresetFile(ThemePreset{
		Name:   name,
		Colors: current.Preset.Colors,
	})
	if err != nil {
		m.themeEditError = err.Error()
		return
	}
	m.reloadThemeEntries(entry.FileName, entry.Preset.ID)
}

const unsavedThemeDiscardPrompt = "Unsaved changes. Press Esc again to discard, or Ctrl+S to save."

// confirmThemeDiscard keeps an accidental close from silently losing edits.
// The second close action explicitly discards the in-memory changes.
func (m *model) confirmThemeDiscard() bool {
	if !m.themeDirty {
		return true
	}
	if m.themeEditError == unsavedThemeDiscardPrompt {
		m.themeDirty = false
		m.themeEditError = ""
		m.themeSavedPreset = ThemePreset{}
		return true
	}
	m.themeEditError = unsavedThemeDiscardPrompt
	return false
}

func (m *model) saveSelectedThemeEdits() {
	if m.themeEditField != "" {
		m.themeEditError = "Confirm or cancel the current field edit first."
		return
	}
	if !m.themeDirty {
		return
	}
	entry, ok := m.selectedThemeEntry()
	if !ok {
		return
	}
	updated, err := saveThemePresetFile(entry, entry.Preset)
	if err != nil {
		m.themeEditError = err.Error()
		return
	}
	m.reloadThemeEntries(updated.FileName, updated.Preset.ID)
}

// decodeThemePackBytes is used by tests and keeps the TOML parsing path shared
// with the on-disk validation performed by loadThemePacks.
func decodeThemePackBytes(content []byte) (ThemePack, error) {
	var pack ThemePack
	if _, err := toml.Decode(string(content), &pack); err != nil {
		return ThemePack{}, err
	}
	return pack, validateThemePack(pack)
}
