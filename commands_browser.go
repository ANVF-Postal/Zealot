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

// Folder tabs have a top rim and a labelled face. The active folder joins
// the handbook content; muted inactive folders sit in the background.
func (m model) commandsTabsLine() (top, face string, ranges []messageBodyRange) {
	bg := lipgloss.Color(m.cfg.Theme.Background)
	active := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).Background(bg).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(bg)
	labels := []string{" CHAT ", " USER ", " LAUNCH "}
	ranges = make([]messageBodyRange,len(labels))
	used := 1
	top = " "
	face = " "
	contentWidth := chatContentWidth(m.width)
	for i,label := range labels {
		tabWidth := lipgloss.Width(label)+2
		if used+tabWidth > contentWidth { break }
		style := inactive
		rim := "┌"+strings.Repeat("─",lipgloss.Width(label))+"┐"
		if i==m.commandsTab {
			style=active
			rim="╭"+strings.Repeat("─",lipgloss.Width(label))+"╮"
		}
		top += style.Render(rim)
		face += style.Render("│"+label+"│")
		ranges[i]=messageBodyRange{Start:used,End:used+tabWidth}
		used+=tabWidth
	}
	return
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
	return max(1, m.messageHeight()-3)
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
	if !m.commandsOpen || (y != 2 && y != 3) {
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
	tabRim, tabFace, _ := m.commandsTabsLine()
	content := m.commandsTextRows()
	thumbTop, thumbHeight, trackHeight := m.commandsScrollbarThumb()
	railAt := func(trackPosition int, bottom bool) string {
		if bottom { return "v" }
		switch {
		case m.maxCommandsScroll()>0 && trackPosition >= thumbTop && trackPosition < thumbTop+thumbHeight: return "█"
		case trackPosition >= trackHeight: return " "
		default: return "│"
		}
	}
	rows := []string{padANSIToWidth(tabRim,width,fill)+railStyle.Render("^")}
	if height > 1 {
		rows=append(rows,padANSIToWidth(tabFace,width,fill)+railStyle.Render(railAt(0,false)))
	}
	if height > 2 {
		rows=append(rows,padANSIToWidth(accent.Render(" "+strings.Repeat("─",max(0,width-1))),width,fill)+railStyle.Render(railAt(1,false)))
	}
	visible := max(0,height-len(rows))
	for i:=0; i<visible; i++ {
		line := ""
		if index := m.commandsScroll+i; index < len(content) { line=content[index] }
		rows=append(rows,padANSIToWidth(line,width,fill)+railStyle.Render(railAt(i+2,i==visible-1)))
	}
	return m.chatBorderStyle().Width(m.width).Height(m.messageHeight()).Render(strings.Join(rows,"\n"))
}
