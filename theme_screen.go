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
	m.clampThemeSelection()
}

func (m *model) themeVisibleRows() int {
	return max(1, m.messageHeight()-2-2) // outer frame, title and separator
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
	m.themeSelected += delta
	m.clampThemeSelection()
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
	contentHeight := max(1, m.messageHeight()-2)
	bg := lipgloss.Color(m.cfg.Theme.Background)
	accentBG := lipgloss.Color(m.cfg.Theme.AccentColor)
	border := lipgloss.Color(m.cfg.Theme.BorderColor)

	leftRows := make([]string, 0, contentHeight)
	leftRows = append(leftRows, m.styles.accent.Render("THEMES"))
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

func (m model) themeDetailsRows(width int) []string {
	if m.themeError != "" {
		return []string{m.styles.accent.Render("THEME LIBRARY"), "", m.styles.system.Render(truncateANSI(m.themeError, width))}
	}
	if len(m.themeEntries) == 0 {
		return []string{m.styles.accent.Render("THEME DETAILS"), "", "No themes were found."}
	}
	entry := m.themeEntries[min(max(0, m.themeSelected), len(m.themeEntries)-1)]
	preset := entry.Preset
	state := "Preview only · Ctrl+Enter to apply"
	if preset.Colors == m.cfg.Theme {
		state = "✓ Currently active"
	}
	rows := []string{
		m.styles.accent.Render(preset.Name),
		m.styles.system.Render("Pack: " + entry.PackName),
		m.styles.system.Render(state),
		"",
		m.styles.accent.Render("COLOR PALETTE"),
	}
	colors := []struct{ label, value string }{
		{"Background", preset.Colors.Background},
		{"Main text", preset.Colors.TextColor},
		{"Borders", preset.Colors.BorderColor},
		{"Accent", preset.Colors.AccentColor},
		{"Hover text", preset.Colors.HoverTextColor},
		{"Muted text", preset.Colors.MutedTextColor},
		{"Mention highlight", preset.Colors.MentionBackgroundColor},
		{"Composer selection", preset.Colors.SelectionBackgroundColor},
	}
	for _, color := range colors {
		swatch := lipgloss.NewStyle().
			Foreground(lipgloss.Color(color.value)).
			Background(lipgloss.Color(color.value)).
			Render("  ")
		rows = append(rows, fmt.Sprintf("%s %-20s %s", swatch, color.label, color.value))
	}
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
}
