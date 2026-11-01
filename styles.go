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

func makeStyles(cfg Config) uiStyles {
	bg := lipgloss.Color(cfg.Background)
	text := lipgloss.Color(cfg.TextColor)
	border := lipgloss.Color(cfg.BorderColor)
	accent := lipgloss.Color(cfg.AccentColor)

	return uiStyles{
		app: lipgloss.NewStyle().
			Background(bg).
			Foreground(text),

		time: lipgloss.NewStyle().
			Background(bg).
			Foreground(lipgloss.Color("#707786")),

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
			Foreground(lipgloss.Color("#707786")),

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
			Foreground(lipgloss.Color("#707786")).
			Italic(true),
	}
}
