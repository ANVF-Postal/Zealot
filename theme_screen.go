package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	return max(1, m.messageHeight()-2-2) // pane border, title and separator
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

// themeBrowserListIndexAt maps a mouse click in the left pane to its selected
// theme. The screen coordinates include the app header and both pane borders.
func (m model) themeBrowserListIndexAt(x, y int) (int, bool) {
	available := chatInnerWidth(m.width)
	leftWidth := themeBrowserLeftWidth(available)
	leftStartX := 1
	firstRowY := 5 // app header, outer pane border, inner pane border, title, separator
	if x < leftStartX+1 || x >= leftStartX+leftWidth-1 || y < firstRowY {
		return 0, false
	}
	index := m.themeScroll + y - firstRowY
	if index < 0 || index >= len(m.themeEntries) || y >= firstRowY+m.themeVisibleRows() {
		return 0, false
	}
	return index, true
}

// renderThemeBrowser replaces the splash pane with the theme list and palette
// details. Browsing a preset is only a preview; Enter is the explicit Apply.
func (m model) renderThemeBrowser() string {
	availableWidth := chatInnerWidth(m.width)
	leftWidth := themeBrowserLeftWidth(availableWidth)
	gapWidth := 1
	rightWidth := max(1, availableWidth-leftWidth-gapWidth)
	contentHeight := max(1, m.messageHeight()-2)
	leftContentWidth := max(1, leftWidth-2)
	rightContentWidth := max(1, rightWidth-2)

	bg := lipgloss.Color(m.cfg.Theme.Background)
	border := lipgloss.Color(m.cfg.Theme.BorderColor)
	leftRows := make([]string, 0, contentHeight)
	leftRows = append(leftRows, m.styles.accent.Render("THEMES"))
	leftRows = append(leftRows, m.styles.system.Render(strings.Repeat("─", leftContentWidth)))

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
			style := lipgloss.NewStyle().Foreground(bg).Background(lipgloss.Color(m.cfg.Theme.AccentColor)).Bold(true)
			leftRows = append(leftRows, fitThemeRow(style.Render(truncateANSI(label, leftContentWidth)), leftContentWidth))
		} else {
			label = "  " + label
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
			leftRows = append(leftRows, fitThemeRow(style.Render(truncateANSI(label, leftContentWidth)), leftContentWidth))
		}
	}
	leftRows = fitThemeRows(leftRows, contentHeight, leftContentWidth)

	rightRows := m.themeDetailsRows(rightContentWidth)
	rightRows = fitThemeRows(rightRows, contentHeight, rightContentWidth)
	paneStyle := func(width int) lipgloss.Style {
		return lipgloss.NewStyle().
			Width(width-2).
			Height(contentHeight).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			BorderBackground(bg).
			Background(bg)
	}
	leftPane := paneStyle(leftWidth).Render(strings.Join(leftRows, "\n"))
	rightPane := paneStyle(rightWidth).Render(strings.Join(rightRows, "\n"))
	gap := lipgloss.NewStyle().Width(gapWidth).Height(m.messageHeight()).Background(bg).Render("")
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, gap, rightPane)
	return m.chatBorderStyle().Width(m.width).Height(m.messageHeight()).Render(content)
}

func fitThemeRows(rows []string, height, width int) []string {
	if len(rows) > height {
		rows = rows[:height]
	}
	for len(rows) < height {
		rows = append(rows, "")
	}
	for i := range rows {
		rows[i] = fitThemeRow(rows[i], width)
	}
	return rows
}

func fitThemeRow(line string, width int) string {
	line = truncateANSI(line, width)
	padding := max(0, width-lipgloss.Width(line))
	if padding > 0 {
		line += strings.Repeat(" ", padding)
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
	state := "Preview only · press Enter to apply"
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
		swatch := lipgloss.NewStyle().Foreground(lipgloss.Color(color.value)).Render("██")
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
