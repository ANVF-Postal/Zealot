package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// themeListEntry flattens a preset from a pack for the two-pane browser.
type themeListEntry struct {
	Preset   ThemePreset
	PackName string
	FileName string
}

func flattenThemePacks(packs []ThemePack) []themeListEntry {
	var entries []themeListEntry
	for _, pack := range packs {
		for _, preset := range pack.Themes {
			entries = append(entries, themeListEntry{
				Preset: preset, PackName: pack.PackName, FileName: pack.FileName,
			})
		}
	}
	return entries
}

func (m *model) openThemeBrowser() {
	packs, err := loadThemePacks()
	m.themesOpen = true
	m.splashOpen = true
	m.usersOpen = false
	m.themeError = ""
	// Opening the theme browser must not steal focus from the always-visible
	// composer; keep the textarea ready for immediate typing.
	m.input.Focus()
	m.themeEntries = nil
	m.themeSelected = 0
	m.themeScroll = 0
	if err != nil {
		m.themeError = err.Error()
		return
	}
	m.themeEntries = flattenThemePacks(packs)
	for i, entry := range m.themeEntries {
		if entry.Preset.Colors == m.cfg.Theme {
			m.themeSelected = i
			break
		}
	}
	m.themeDirty = false
	m.themeEditField = ""
	m.themeEditError = ""
	if len(m.themeEntries) > 0 {
		m.themeSavedPreset = m.themeEntries[m.themeSelected].Preset
	}
	m.clampThemeSelection()
}

func (m *model) themeVisibleRows() int {
	// The title and separator occupy two rows inside the chat viewport.
	// The viewport height itself already excludes the outer frame.
	return max(1, m.messageHeight()-2)
}

func (m *model) clampThemeSelection() {
	if len(m.themeEntries) == 0 {
		m.themeSelected = 0
		m.themeScroll = 0
		return
	}
	m.themeSelected = min(max(0, m.themeSelected), len(m.themeEntries)-1)
	visible := m.themeVisibleRows()
	maxScroll := max(0, len(m.themeEntries)-visible)
	if m.themeSelected < m.themeScroll {
		m.themeScroll = m.themeSelected
	}
	if m.themeSelected >= m.themeScroll+visible {
		m.themeScroll = m.themeSelected-visible+1
	}
	m.themeScroll = min(max(0, m.themeScroll), maxScroll)
}

func (m *model) moveThemeSelection(delta int) {
	if len(m.themeEntries) == 0 {
		return
	}
	if m.themeDirty || m.themeEditField != "" {
		m.themeEditError = "Save the current edits before selecting another theme."
		return
	}
	m.themeSelected += delta
	m.clampThemeSelection()
	if len(m.themeEntries) > 0 {
		m.themeSavedPreset = m.themeEntries[m.themeSelected].Preset
	}
}

func themeBrowserLeftWidth(total int) int {
	if total < 27 {
		return max(1, total/2)
	}
	return min(30, max(14, total/3))
}

// themeBrowserLeftBounds keeps both columns usable while allowing the divider
// to move. The available width excludes the one-cell divider.
func themeBrowserLeftBounds(total int) (int, int) {
	if total <= 2 {
		return 1, 1
	}
	minLeft := min(12, max(1, total/3))
	minRight := min(16, max(1, total/3))
	if minLeft+minRight > total {
		minLeft = max(1, total/2)
		minRight = max(1, total-minLeft)
	}
	maxLeft := max(minLeft, total-minRight)
	return minLeft, maxLeft
}

func (m model) themeBrowserColumnWidths() (int, int) {
	columns := max(1, chatInnerWidth(m.width)-1) // reserve the divider cell
	left := m.themeLeftWidth
	if left <= 0 {
		left = themeBrowserLeftWidth(columns)
	}
	minLeft, maxLeft := themeBrowserLeftBounds(columns)
	left = min(max(left, minLeft), maxLeft)
	return left, max(1, columns-left)
}

func (m model) themeBrowserContentStartX() int {
	if narrowBordersHidden(m.width) {
		return 0
	}
	return 1
}

func (m model) themeBrowserDividerX() int {
	left, _ := m.themeBrowserColumnWidths()
	return m.themeBrowserContentStartX() + left
}

func (m model) themeBrowserDividerContains(x, y int) bool {
	return m.themesOpen &&
		x >= m.themeBrowserDividerX()-1 && x <= m.themeBrowserDividerX()+1 &&
		y >= 2 && y < 1+m.messageHeight()
}

func (m *model) setThemeBrowserDivider(x int) {
	columns := max(1, chatInnerWidth(m.width)-1)
	minLeft, maxLeft := themeBrowserLeftBounds(columns)
	m.themeLeftWidth = min(max(x-m.themeBrowserContentStartX(), minLeft), maxLeft)
}

// themeBrowserListIndexAt maps a click in the left column to its selected
// theme. The one-cell divider separates it from the details column.
func (m model) themeBrowserListIndexAt(x, y int) (int, bool) {
	leftWidth, _ := m.themeBrowserColumnWidths()
	leftStartX := m.themeBrowserContentStartX()
	firstRowY := 4 // header, outer frame, title, separator
	if x < leftStartX || x >= leftStartX+leftWidth || y < firstRowY {
		return 0, false
	}
	index := m.themeScroll + y - firstRowY
	if index < 0 || index >= len(m.themeEntries) || y >= firstRowY+m.themeVisibleRows() {
		return 0, false
	}
	return index, true
}

// renderThemeBrowser uses the chat frame as its only border. The two columns
// are aligned by a single, draggable divider. Browsing remains preview-only;
// Ctrl+Enter explicitly applies the selected preset.
func (m model) renderThemeBrowser() string {
	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	contentHeight := max(1, m.messageHeight())
	bg := lipgloss.Color(m.cfg.Theme.Background)
	accentBG := lipgloss.Color(m.cfg.Theme.AccentColor)
	border := lipgloss.Color(m.cfg.Theme.BorderColor)

	leftRows := make([]string, 0, contentHeight)
	header := m.styles.accent.Render("THEMES")
	create := m.styles.accent.Render(themeCreateButtonLabel)
	headerGap := max(1, leftWidth-lipgloss.Width(header)-lipgloss.Width(create))
	headerFill := lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", headerGap))
	leftRows = append(leftRows, fitThemeRowWithBackground(header+headerFill+create, leftWidth, bg))
	leftRows = append(leftRows, m.styles.system.Render(strings.Repeat("─", leftWidth)))

	visible := m.themeVisibleRows()
	for row := 0; row < visible; row++ {
		index := m.themeScroll + row
		if index >= len(m.themeEntries) {
			leftRows = append(leftRows, "")
			continue
		}
		entry := m.themeEntries[index]
		label := entry.Preset.Name
		if entry.Preset.Colors == m.cfg.Theme {
			label += "  *"
		}
		if index == m.themeSelected {
			label = "› " + label
			style := lipgloss.NewStyle().Foreground(bg).Background(accentBG).Bold(true)
			leftRows = append(leftRows, fitThemeRowWithBackground(style.Render(truncateANSI(label, leftWidth)), leftWidth, accentBG))
		} else {
			label = "  " + label
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
			leftRows = append(leftRows, fitThemeRowWithBackground(style.Render(truncateANSI(label, leftWidth)), leftWidth, bg))
		}
	}
	leftRows = fitThemeRows(leftRows, contentHeight, leftWidth, bg)

	rightRows := m.themeDetailsRows(rightWidth)
	rightRows = fitThemeRows(rightRows, contentHeight, rightWidth, bg)

	dividerStyle := lipgloss.NewStyle().Foreground(border).Background(bg)
	dividerRows := make([]string, contentHeight)
	for i := range dividerRows {
		dividerRows[i] = dividerStyle.Render("│")
	}
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		strings.Join(leftRows, "\n"),
		strings.Join(dividerRows, "\n"),
		strings.Join(rightRows, "\n"),
	)
	return m.chatBorderStyle().Width(m.width).Height(m.messageHeight()).Render(content)
}

func fitThemeRows(rows []string, height, width int, background ...color.Color) []string {
	if len(rows) > height {
		rows = rows[:height]
	}
	for len(rows) < height {
		rows = append(rows, "")
	}
	for i := range rows {
		if len(background) > 0 {
			rows[i] = fitThemeRowWithBackground(rows[i], width, background[0])
		} else {
			rows[i] = fitThemeRow(rows[i], width)
		}
	}
	return rows
}

func fitThemeRow(line string, width int) string {
	return fitThemeRowWithBackground(line, width, nil)
}

func fitThemeRowWithBackground(line string, width int, background color.Color) string {
	line = truncateANSI(line, width)
	padding := max(0, width-lipgloss.Width(line))
	if padding > 0 {
		spaces := strings.Repeat(" ", padding)
		if background != nil {
			spaces = lipgloss.NewStyle().Background(background).Render(spaces)
		}
		line += spaces
	}
	return line
}

const (
	themeCreateButtonLabel = "[+]"
	themeCloneButtonLabel  = "[CLONE]"
	themeSaveButtonLabel   = "[SAVE]"
	themeApplyButtonLabel  = "[APPLY]"
	themePaletteFirstRow   = 5
	themePaletteColorCount = 8
)

func themeCloneButtonRow() int {
	return themePaletteFirstRow + themePaletteColorCount + 1
}

func themeSaveButtonRow() int {
	return themeCloneButtonRow() + 1
}

// themeApplyButtonRow is the stable row for the clean (not-yet-edited) editor.
// Dirty editors insert a Save row immediately before Apply.
func themeApplyButtonRow() int {
	return themeCloneButtonRow() + 1
}

func (m model) themeApplyButtonRowIndex() int {
	if m.themeDirty {
		return themeSaveButtonRow() + 1
	}
	return themeApplyButtonRow()
}

func themeCenteredButtonRow(label string, width int, background, accent string) string {
	button := lipgloss.NewStyle().
		Foreground(lipgloss.Color(background)).
		Background(lipgloss.Color(accent)).
		Bold(true).
		Padding(0, 1).
		Render(label)
	padding := max(0, (width-lipgloss.Width(button))/2)
	fill := lipgloss.NewStyle().Background(lipgloss.Color(background))
	left := fill.Render(strings.Repeat(" ", padding))
	return fitThemeRowWithBackground(left+button, width, lipgloss.Color(background))
}

func (m model) themeActionButtonContains(label string, x, y int) bool {
	if !m.themesOpen || m.themeError != "" || len(m.themeEntries) == 0 {
		return false
	}
	row := -1
	switch label {
	case themeCloneButtonLabel:
		row = themeCloneButtonRow()
	case themeSaveButtonLabel:
		if m.themeDirty {
			row = themeSaveButtonRow()
		}
	case themeApplyButtonLabel:
		row = m.themeApplyButtonRowIndex()
	}
	if row < 0 || row >= max(1, m.messageHeight()) || y != 2+row {
		return false
	}
	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	rightStartX := m.themeBrowserContentStartX() + leftWidth + 1
	buttonWidth := lipgloss.Width(label) + 2
	buttonStartX := rightStartX + max(0, (rightWidth-buttonWidth)/2)
	return x >= buttonStartX && x < buttonStartX+buttonWidth
}

func (m model) themeApplyButtonContains(x, y int) bool {
	return m.themeActionButtonContains(themeApplyButtonLabel, x, y)
}

func (m model) themeCloneButtonContains(x, y int) bool {
	return m.themeActionButtonContains(themeCloneButtonLabel, x, y)
}

func (m model) themeSaveButtonContains(x, y int) bool {
	return m.themeActionButtonContains(themeSaveButtonLabel, x, y)
}

func (m model) themeCreateButtonContains(x, y int) bool {
	if !m.themesOpen || m.themeError != "" || y != 2 {
		return false
	}
	leftWidth, _ := m.themeBrowserColumnWidths()
	leftStartX := m.themeBrowserContentStartX()
	buttonStartX := leftStartX + leftWidth - lipgloss.Width(themeCreateButtonLabel)
	return x >= buttonStartX && x < leftStartX+leftWidth
}

func (m model) themeNameContains(x, y int) bool {
	if !m.themesOpen || m.themeError != "" || len(m.themeEntries) == 0 || y != 2 {
		return false
	}
	entry := m.themeEntries[min(max(0, m.themeSelected), len(m.themeEntries)-1)]
	if !themeEntryEditable(entry) {
		return false
	}
	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	rightStartX := m.themeBrowserContentStartX() + leftWidth + 1
	titleWidth := lipgloss.Width(entry.Preset.Name + "  ✎")
	titleWidth = min(titleWidth, rightWidth)
	return x >= rightStartX && x < rightStartX+titleWidth
}

func (m model) themeColorFieldAt(x, y int) (string, bool) {
	if !m.themesOpen || m.themeError != "" || len(m.themeEntries) == 0 {
		return "", false
	}
	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	rightStartX := m.themeBrowserContentStartX() + leftWidth + 1
	if x < rightStartX || x >= rightStartX+rightWidth {
		return "", false
	}
	colorIndex := y - 2 - themePaletteFirstRow
	if colorIndex < 0 || colorIndex >= themePaletteColorCount {
		return "", false
	}
	fields := []string{
		"background",
		"text_color",
		"border_color",
		"accent_color",
		"hover_text_color",
		"muted_text_color",
		"mention_background_color",
		"selection_background_color",
	}
	return fields[colorIndex], true
}

func (m model) themeDetailsRows(width int) []string {
	if m.themeError != "" {
		return []string{m.styles.accent.Render("THEME LIBRARY"), "", m.styles.system.Render(truncateANSI(m.themeError, width))}
	}
	if len(m.themeEntries) == 0 {
		return []string{m.styles.accent.Render("THEME DETAILS"), "", "No themes were found."}
	}
	entry := m.themeEntries[min(max(0, m.themeSelected), len(m.themeEntries)-1)]
	preset := entry.Preset
	editable := themeEntryEditable(entry)
	title := preset.Name
	state := "Built-in · read-only · clone to edit"
	if editable {
		title += "  ✎"
		state = "Custom theme · click name or a color to edit"
	}
	if preset.Colors == m.cfg.Theme {
		if editable {
			state = "✓ Active custom theme"
		} else {
			state = "✓ Active built-in theme · read-only"
		}
	}
	if m.themeEditField != "" {
		state = "Editing " + themeFieldLabel(m.themeEditField) + " · Enter confirms · Esc cancels"
	}
	if m.themeDirty && m.themeEditField == "" {
		state = "Unsaved changes · click [SAVE] or press Ctrl+S"
	}
	if m.themeEditError != "" {
		state = truncateANSI(m.themeEditError, width)
	}
	rows := []string{
		m.styles.accent.Render(title),
		m.styles.system.Render("Pack: " + entry.PackName),
		m.styles.system.Render(state),
		"",
		m.styles.accent.Render("COLOR PALETTE · click a row to edit"),
	}
	colors := []struct{ field, label, value string }{
		{"background", "Background", preset.Colors.Background},
		{"text_color", "Main text", preset.Colors.TextColor},
		{"border_color", "Borders", preset.Colors.BorderColor},
		{"accent_color", "Accent", preset.Colors.AccentColor},
		{"hover_text_color", "Hover text", preset.Colors.HoverTextColor},
		{"muted_text_color", "Muted text", preset.Colors.MutedTextColor},
		{"mention_background_color", "Mention highlight", preset.Colors.MentionBackgroundColor},
		{"selection_background_color", "Composer selection", preset.Colors.SelectionBackgroundColor},
	}
	for _, color := range colors {
		swatch := lipgloss.NewStyle().
			Background(lipgloss.Color(color.value)).
			Render("  ")
		paletteStyle := m.styles.app
		if m.themeEditField == color.field {
			paletteStyle = m.styles.accent
		}
		paletteText := paletteStyle.Render(fmt.Sprintf(" %-20s %s", color.label, color.value))
		rows = append(rows, swatch+paletteText)
	}
	rows = append(rows, "")
	rows = append(rows, themeCenteredButtonRow(themeCloneButtonLabel, width, m.cfg.Theme.Background, m.cfg.Theme.AccentColor))
	if m.themeDirty {
		rows = append(rows, themeCenteredButtonRow(themeSaveButtonLabel, width, m.cfg.Theme.Background, m.cfg.Theme.AccentColor))
	}
	rows = append(rows, themeCenteredButtonRow(themeApplyButtonLabel, width, m.cfg.Theme.Background, m.cfg.Theme.AccentColor))
	return rows
}

// applySelectedTheme commits the selected preset, then refreshes all live UI
// styles without rebuilding the model or disturbing the current composer.
func (m *model) applySelectedTheme() {
	if len(m.themeEntries) == 0 {
		return
	}
	entry := m.themeEntries[min(max(0, m.themeSelected), len(m.themeEntries)-1)]
	if err := applyThemePreset(&m.cfg, entry.Preset); err != nil {
		m.themeError = err.Error()
		return
	}
	m.themeError = ""
	m.styles = makeStyles(m.cfg.Theme)

	bg := lipgloss.Color(m.cfg.Theme.Background)
	text := lipgloss.Color(m.cfg.Theme.TextColor)
	base := lipgloss.NewStyle().Foreground(text).Background(bg)
	selection := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Theme.SelectionBackgroundColor))
	inputStyles := m.input.Styles()
	inputStyles.Focused.Base = base
	inputStyles.Blurred.Base = base
	inputStyles.Focused.CursorLine = base
	inputStyles.Blurred.CursorLine = base
	inputStyles.Focused.EndOfBuffer = base
	inputStyles.Blurred.EndOfBuffer = base
	inputStyles.Focused.Text = base
	inputStyles.Blurred.Text = base
	inputStyles.Focused.Selection = selection
	inputStyles.Blurred.Selection = selection
	placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(bg).Faint(false).Bold(false).Italic(false)
	inputStyles.Focused.Placeholder = placeholder
	inputStyles.Blurred.Placeholder = placeholder
	inputStyles.Cursor.Color = lipgloss.Color(m.cfg.Theme.AccentColor)
	m.input.SetStyles(inputStyles)
	// Message rows are cached as ANSI-styled strings. Rebuild the visible
	// window immediately so existing history adopts the new palette at once.
	m.rebuildMessageCache()
}
