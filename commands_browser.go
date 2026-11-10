package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The handbook is deliberately a public command reference. Only commands
// explicitly listed here appear on screen.
type handbookEntry struct {
	Command string
	Description string
}

var handbookPages = []struct {
	Name string
	Entries []handbookEntry
}{
	{"CHAT", []handbookEntry{
		{"@user", "Lets you send a message that will be highlighted on the specified user's screen."},
		{"/me", "Sends a message written in the third person."},
		{"/rainbow", "Toggle: messages you send will be in rainbow colors."},
		{"/rb", "Sends a single message in rainbow colors."},
		{"/coinflip", "Sends a special message with the result of a coin flip."},
		{"/random 1-100", "Picks a random number between 1 and 100. You can specify your own minimum and maximum."},
		{"/long", "Lets you write a long message (up to 100,000 characters). It appears in chat as [Long Message], which you can click to open."},
		{"/spoiler", "Obfuscates the following text as # symbols. Click the spoiler message to reveal the text."},
		{"Click a message", "Click another user's message to reply directly to it."},
		{"Click a username", "Click a user's name to insert an @mention with their name pre-written."},
	}},
	{"USER", []handbookEntry{
		{"/rename", "Renames you from the chat window. Usage: /rename name"},
		{"/color", "Changes your user color from the chat window. Use #RRGGBB hexadecimal or an ANSI color number (0–255)."},
		{"/who", "Prints the users connected to the current room. Only you see this list."},
		{"/history", "Shows whether chat history is enabled for the current room."},
	}},
	{"LAUNCH", []handbookEntry{
		{"--room example", "Joins the room named example, creating it if it doesn't exist."},
		{"--add-history", "Enables chat history when creating a room. New arrivals can see earlier messages. History clears when the last user leaves."},
	}},
}

func (m *model) openCommandsBrowser() {
	m.commandsOpen = true
	m.splashOpen = true
	m.themesOpen = false
	m.commandsTab = 0
	m.commandsScroll = 0
	m.commandsDragging = false
	m.commandsHoldDirection = 0
	m.commandsHoldGeneration++
	m.input.Focus()
}

func (m *model) closeCommandsBrowser() {
	m.commandsOpen = false
	m.commandsDragging = false
	m.commandsHoldDirection = 0
	m.commandsHoldGeneration++
	m.splashScroll = 0
	m.clampSplashScroll()
}

func (m *model) selectCommandsTab(index int) {
	if index < 0 || index >= len(handbookPages) || index == m.commandsTab {
		return
	}
	m.commandsTab = index
	m.commandsScroll = 0
	m.commandsDragging = false
	m.commandsHoldDirection = 0
	m.commandsHoldGeneration++
}

func (m model) commandsTabLabels() []string {
	return []string{" CHAT ", " USER ", " LAUNCH "}
}

func (m model) commandsTabsLine() (string, []messageBodyRange) {
	bg := lipgloss.Color(m.cfg.Theme.Background)
	accent := lipgloss.Color(m.cfg.Theme.AccentColor)
	muted := lipgloss.Color(m.cfg.Theme.MutedTextColor)
	contentWidth := max(1, chatContentWidth(m.width))
	labels := m.commandsTabLabels()
	ranges := make([]messageBodyRange, len(labels))
	var out strings.Builder
	used := 1
	out.WriteString(" ")
	for i, label := range labels {
		// Folder shape: raised selected tab and recessed inactive tabs.
		text := "┌" + strings.Repeat("─", lipgloss.Width(label)) + "┐"
		if i == m.commandsTab {
			text = "╭" + strings.Repeat("─", lipgloss.Width(label)) + "╮"
		}
		_ = text // the tab's face uses the matching horizontal folder edge
		tab := "[" + strings.TrimSpace(label) + "]"
		if used+lipgloss.Width(tab)+1 > contentWidth {
			break
		}
		ranges[i] = messageBodyRange{Start: used, End: used + lipgloss.Width(tab)}
		style := lipgloss.NewStyle().Background(bg).Foreground(muted)
		if i == m.commandsTab {
			style = style.Foreground(accent).Bold(true).Underline(true)
		}
		out.WriteString(style.Render(tab))
		out.WriteString(" ")
		used += lipgloss.Width(tab)+1
	}
	return out.String(), ranges
}

func (m model) commandsTextRows() []string {
	width := max(1, chatContentWidth(m.width)-3)
	bg := lipgloss.Color(m.cfg.Theme.Background)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg).Bold(true)
	descriptionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(bg)
	lines := make([]string, 0, 40)
	for _, entry := range handbookPages[m.commandsTab].Entries {
		for _, line := range wrapText(entry.Command, width) {
			lines = append(lines, nameStyle.Render("  "+line))
		}
		for _, line := range wrapText(entry.Description, width) {
			lines = append(lines, descriptionStyle.Render("   "+line))
		}
		lines = append(lines, "")
	}
	return lines
}

func (m model) commandsVisibleHeight() int {
	return max(1, m.messageHeight()-2)
}

func (m model) maxCommandsScroll() int {
	return max(0, len(m.commandsTextRows())-m.commandsVisibleHeight())
}

func (m *model) clampCommandsScroll() {
	m.commandsScroll = min(max(0, m.commandsScroll), m.maxCommandsScroll())
}

func (m model) commandsScrollbarThumb() (top, thumbHeight, trackHeight int) {
	return scrollbarGeometry(len(m.commandsTextRows()), m.commandsVisibleHeight(), m.messageHeight()-2, m.commandsScroll)
}

func (m *model) setCommandsScrollFromThumbTop(top int) {
	_, thumbHeight, track := m.commandsScrollbarThumb()
	m.commandsScroll = scrollbarScrollForThumb(top, len(m.commandsTextRows()), m.commandsVisibleHeight(), track, thumbHeight)
	m.clampCommandsScroll()
}

func (m *model) setCommandsScrollFromTrack(trackPos int) {
	_, height, _ := m.commandsScrollbarThumb()
	m.setCommandsScrollFromThumbTop(trackPos-height/2)
}

func (m model) commandsHoldCmd(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return commandsScrollbarHoldMsg{generation: generation}
	})
}

func (m model) commandsTabAt(x, y int) int {
	if !m.commandsOpen || y != 2 {
		return -1
	}
	left := 0
	if !narrowBordersHidden(m.width) { left = 1 }
	_, ranges := m.commandsTabsLine()
	for index, hit := range ranges {
		if hit.End > hit.Start && x-left >= hit.Start && x-left < hit.End {
			return index
		}
	}
	return -1
}

func (m model) renderCommandsBrowser() string {
	bg := lipgloss.Color(m.cfg.Theme.Background)
	fill := lipgloss.NewStyle().Background(bg)
	railStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.BorderColor)).Background(bg)
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).Background(bg)
	width := chatContentWidth(m.width)
	height := max(1, m.messageHeight())
	tabs, _ := m.commandsTabsLine()
	rows := []string{padANSIToWidth(tabs, width, fill)+railStyle.Render("^")}
	if height > 1 {
		rows = append(rows, padANSIToWidth(accent.Render("  └"+strings.Repeat("─", max(0,width-3))+"┘"),width,fill)+railStyle.Render("│"))
	}
	content := m.commandsTextRows()
	thumbTop, thumbHeight, trackHeight := m.commandsScrollbarThumb()
	visible := max(0,height-len(rows))
	for i:=0; i<visible; i++ {
		line := ""
		if index := m.commandsScroll+i; index < len(content) { line=content[index] }
		rail := "│"
		switch {
		case i == visible-1: rail="v"
		case m.maxCommandsScroll()>0 && i+1 >= thumbTop && i+1 < thumbTop+thumbHeight: rail="█"
		case i+1>=trackHeight: rail=" "
		}
		rows=append(rows,padANSIToWidth(line,width,fill)+railStyle.Render(rail))
	}
	return m.chatBorderStyle().Width(m.width).Height(m.messageHeight()).Render(strings.Join(rows,"\n"))
}
