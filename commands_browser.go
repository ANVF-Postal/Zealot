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
		{"/poll question | option1 | option2", "Creates a single-choice poll with 2–6 options. Click your selected option again to retract your vote."},
		{"/multipoll question | option1 | option2", "Creates a poll allowing multiple selections. Click an option again to remove it."},
		{"/vote ID OPTION", "Votes for an option in poll ID; you may change your choice until it closes."},
		{"/poll close ID", "Closes one of your polls and reveals final results."},
		{"/whisper @user message", "Sends a private message visible only to you and the specified user. Whispers are not saved in public room history."},
		{"/rainbow", "Toggle: messages you send will be in rainbow colors."},
		{"/rb", "Sends a single message in rainbow colors."},
		{"/coinflip", "Sends a special message with the result of a coin flip."},
		{"/roll 2d6", "Rolls two six-sided dice. Use NdM notation, such as 1d20 or 3d8, for other rolls."},
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
	m.cacheCommandsTextRows()
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

// Folder tabs use the full width when it is available. On compact
// terminals the selected folder stays visible while a fixed, opaque
// < | > overlay covers the right end of the strip (never the scrollbar).
// The hitboxes below are derived from these same visible cell positions.
const commandsTabNavigation = "< | >"

func commandsTabWidth(label string) int {
	return lipgloss.Width(label) + 2 // folder sides
}

func (m model) commandsTabsOverflow() bool {
	// Match the actual folder artwork, including the leading inset cell.
	total := 1
	for _, label := range []string{" CHAT ", " USER ", " LAUNCH "} {
		total += commandsTabWidth(label)
	}
	return total > chatContentWidth(m.width)
}

func (m model) commandsNavigationAt(x, y int) int {
	if !m.commandsOpen || !m.commandsTabsOverflow() || (y != 2 && y != 3) {
		return 0
	}
	left := 0
	if !narrowBordersHidden(m.width) {
		left = 1
	}
	contentWidth := chatContentWidth(m.width)
	overlayWidth := min(lipgloss.Width(commandsTabNavigation), contentWidth)
	start := left + contentWidth - overlayWidth
	switch x {
	case start:
		return -1
	case start + overlayWidth - 1:
		return 1
	default:
		return 0
	}
}

// commandsTabsLine returns the two folder-art rows and their *visible*
// per-tab hitboxes. When compact, the controls are painted last over the
// rightmost cells, using the configured background to hide clipped tabs.
func (m model) commandsTabsLine() (top, face string, ranges []messageBodyRange) {
	bg := lipgloss.Color(m.cfg.Theme.Background)
	active := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).Background(bg).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(bg)
	labels := []string{" CHAT ", " USER ", " LAUNCH "}
	ranges = make([]messageBodyRange, len(labels))
	width := chatContentWidth(m.width)
	overflow := m.commandsTabsOverflow()
	overlayWidth := 0
	if overflow {
		overlayWidth = min(lipgloss.Width(commandsTabNavigation), width)
	}
	visibleWidth := max(0, width-overlayWidth)

	// Scroll the *tab strip*, not its content. As the user advances,
	// show the selected folder even if earlier folders must slide away.
	first := 0
	if overflow {
		used := 1
		for i := 0; i <= m.commandsTab && i < len(labels); i++ {
			used += commandsTabWidth(labels[i])
		}
		for used > visibleWidth && first < m.commandsTab {
			used -= commandsTabWidth(labels[first])
			first++
		}
	}
	top, face = " ", " "
	used := 1
	for i := first; i < len(labels); i++ {
		label := labels[i]
		tabWidth := commandsTabWidth(label)
		if used >= visibleWidth {
			break
		}
		style := inactive
		rim := "┌" + strings.Repeat("─", lipgloss.Width(label)) + "┐"
		if i == m.commandsTab {
			style = active
			rim = "╭" + strings.Repeat("─", lipgloss.Width(label)) + "╮"
		}
		shown := min(tabWidth, visibleWidth-used)
		top += truncateANSI(style.Render(rim), shown)
		face += truncateANSI(style.Render("│"+label+"│"), shown)
		ranges[i] = messageBodyRange{Start: used, End: used+shown}
		used += shown
	}

	if overflow {
		// Fill the space under the overlay with the theme background
		// so the controls never inherit terminal-default colors.
		top = padANSIToWidth(top, visibleWidth, lipgloss.NewStyle().Background(bg))
		face = padANSIToWidth(face, visibleWidth, lipgloss.NewStyle().Background(bg))
		controls := commandsTabNavigation
		if overlayWidth < lipgloss.Width(controls) {
			controls = truncateANSI(controls, overlayWidth)
		}
		top += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", overlayWidth))
		face += active.Render(controls)
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
	_, _, ranges := m.commandsTabsLine()
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
