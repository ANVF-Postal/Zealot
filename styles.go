package main

import (
	"charm.land/lipgloss/v2"
)

// -----------------------------------------------------------------------------
// UI styles
// -----------------------------------------------------------------------------

type uiStyles struct {
	app      lipgloss.Style
	time     lipgloss.Style
	inputBox lipgloss.Style
	status   lipgloss.Style
	border   lipgloss.Style
	accent   lipgloss.Style
	system   lipgloss.Style
}

func makeStyles(theme Theme) uiStyles {
	bg := lipgloss.Color(theme.Background)
	text := lipgloss.Color(theme.TextColor)
	border := lipgloss.Color(theme.BorderColor)
	accent := lipgloss.Color(theme.AccentColor)
	muted := lipgloss.Color(theme.MutedTextColor)

	return uiStyles{
		app: lipgloss.NewStyle().
			Background(bg).
			Foreground(text),

		time: lipgloss.NewStyle().
			Background(bg).
			Foreground(muted),

		inputBox: lipgloss.NewStyle().
			Background(bg).
			Foreground(text).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			BorderBackground(bg).
			PaddingLeft(1).
			PaddingRight(1),

		status: lipgloss.NewStyle().
			Background(bg).
			Foreground(muted),

		border: lipgloss.NewStyle().
			Background(bg).
			Foreground(text).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			BorderBackground(bg),

		accent: lipgloss.NewStyle().
			Background(bg).
			Foreground(accent).
			Bold(true),

		system: lipgloss.NewStyle().
			Background(bg).
			Foreground(muted).
			Italic(true),
	}
}


// narrowBordersHidden reserves one extra step before compact message headers
// activate, then keeps the side borders hidden at every narrower width.
func narrowBordersHidden(width int) bool {
	return width <= 49
}

// chatInnerWidth is the full interior width of the chat pane, including its
// final scrollbar column.
func chatInnerWidth(width int) int {
	inner := width - 2 // normal left and right borders
	if narrowBordersHidden(width) {
		inner += 2
	}
	return max(1, inner)
}

// chatContentWidth excludes the permanent scrollbar column.
func chatContentWidth(width int) int {
	return max(1, chatInnerWidth(width)-1)
}

func (m model) chatBorderStyle() lipgloss.Style {
	style := m.styles.border
	if narrowBordersHidden(m.width) {
		style = style.BorderLeft(false).BorderRight(false)
	}
	return style
}

func (m model) inputBoxStyle() lipgloss.Style {
	style := m.styles.inputBox
	if narrowBordersHidden(m.width) {
		style = style.BorderLeft(false).BorderRight(false)
	}
	return style
}

func (m model) chatScrollbarX() int {
	if narrowBordersHidden(m.width) {
		return m.width - 1
	}
	return m.width - 2
}
