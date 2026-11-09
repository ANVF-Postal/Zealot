package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"unicode"

	"charm.land/lipgloss/v2"
)

// Long bodies remain in room memory, but only the short link is part of chat
// layout. The expanded reader has a separate bounded viewport.
const maxLongMessage = 100000

func isSpecialMessage(packet Packet) bool {
	return packet.Type == "message" && (packet.Long || packet.Spoiler)
}

func spoilerMask(text string) string {
	var result strings.Builder
	for _, r := range text {
		if r == ' ' || r == '\n' {
			result.WriteRune(r)
			continue
		}
		if r == '\t' {
			result.WriteString("    ")
			continue
		}
		width := lipgloss.Width(string(r))
		if width < 1 {
			width = 1
		}
		result.WriteString(strings.Repeat("#", width))
	}
	return result.String()
}

func terminalSafeText(text string) string {
	// A long reader and revealed spoiler are plain text. Never transmit
	// raw terminal controls such as OSC clipboard commands to the renderer.
	var result strings.Builder
	for _, r := range text {
		if r == '\n' || r == '\t' {
			result.WriteRune(r)
		} else if unicode.IsControl(r) {
			result.WriteRune('�')
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func (m model) displayedMessage(packet Packet) Packet {
	if packet.Type != "message" {
		return packet
	}
	if packet.Long {
		packet.Text = "[Long Message]"
		packet.Rainbow = false
	} else if packet.Spoiler {
		if !m.revealedSpoilers[packet.ID] {
			packet.Text = spoilerMask(packet.Text)
		} else {
			packet.Text = terminalSafeText(packet.Text)
		}
		packet.Rainbow = false
	}
	return packet
}

func (m *model) revealSpoiler(id string) {
	if id == "" {
		return
	}
	if m.revealedSpoilers == nil {
		m.revealedSpoilers = make(map[string]bool)
	}
	m.revealedSpoilers[id] = true
	m.messageBodyRangesByID = nil
	m.messageLinkRangesByID = nil
	m.rebuildMessageCache()
	m.clampScroll()
}

func (m *model) openLongReader(packet Packet) {
	if !packet.Long || packet.Type != "message" {
		return
	}
	m.longReader = &packet
	m.longReaderScroll = 0
	m.longReaderWidth = -1
	m.longReaderDragging = false
	m.longReaderScrollbarHoldDirection = 0
	m.longReaderScrollbarHoldGeneration++
	m.prepareLongReader()
}

// closeLongReader discards the alternate reader; navigating to another panel
// should always return to main chat when that panel is closed.
func (m *model) closeLongReader() {
	m.longReader = nil
	m.longReaderLines = nil
	m.longReaderScroll = 0
	m.longReaderWidth = -1
	m.longReaderDragging = false
	m.longReaderScrollbarHoldDirection = 0
	m.longReaderScrollbarHoldGeneration++
	m.refreshMessageHover()
}

func (m model) longReaderCloseX() int {
	// The reader's [X] sits inside the chat pane, immediately left of
	// the scrollbar rail, matching the other panels' top-right control.
	left := 0
	if !narrowBordersHidden(m.width) {
		left = 1
	}
	return left + max(0, chatContentWidth(m.width)-lipgloss.Width("[X]"))
}

func (m *model) prepareLongReader() {
	if m.longReader == nil || m.longReaderWidth == chatContentWidth(m.width) {
		return
	}
	// Use the same word-aware wrapping as normal chat messages. Explicit
	// paragraph breaks remain visible, including blank lines.
	width := max(1, chatContentWidth(m.width)-1)
	text := strings.ReplaceAll(terminalSafeText(m.longReader.Text), "\t", "    ")
	lines := wrapText(text, width)
	m.longReaderLines = lines
	m.longReaderWidth = chatContentWidth(m.width)
	m.clampLongReaderScroll()
}

func (m model) longReaderVisibleHeight() int {
	return max(1, m.messageHeight()-2)
}

func (m model) maxLongReaderScroll() int {
	return max(0, len(m.longReaderLines)-m.longReaderVisibleHeight())
}

func (m *model) clampLongReaderScroll() {
	m.longReaderScroll = max(0, min(m.longReaderScroll, m.maxLongReaderScroll()))
}

// The long reader uses the same on-screen scrollbar coordinate system as
// main chat: top arrow y=2, draggable track y=3..height, bottom arrow at
// y=1+messageHeight, with the same thumb sizing and positioning helpers.
func (m model) longReaderScrollbarContains(x, y int) bool {
	return m.longReader != nil && x == m.chatScrollbarX() &&
		y >= 2 && y <= 1+m.messageHeight()
}

func (m model) longReaderScrollbarThumb() (top, thumbHeight, trackHeight int) {
	return scrollbarGeometry(
		len(m.longReaderLines), m.longReaderVisibleHeight(),
		m.messageHeight()-2, m.longReaderScroll,
	)
}

func (m *model) setLongReaderScrollFromThumbTop(top int) {
	_, thumbHeight, trackHeight := m.longReaderScrollbarThumb()
	m.longReaderScroll = scrollbarScrollForThumb(
		top, len(m.longReaderLines), m.longReaderVisibleHeight(),
		trackHeight, thumbHeight,
	)
	m.clampLongReaderScroll()
}

func (m *model) setLongReaderScrollFromTrack(trackPosition int) {
	_, thumbHeight, _ := m.longReaderScrollbarThumb()
	m.setLongReaderScrollFromThumbTop(trackPosition - thumbHeight/2)
}

func (m model) longReaderScrollbarHoldCmd(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return longReaderScrollbarHoldMsg{generation: generation}
	})
}

func (m model) renderLongReaderPanel() string {
	bg := lipgloss.Color(m.cfg.Theme.Background)
	fill := lipgloss.NewStyle().Background(bg)
	railStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.BorderColor)).
		Background(bg)
	width := max(1, chatInnerWidth(m.width))
	contentWidth := max(1, width-1)
	height := max(1, m.messageHeight())
	readerName := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(terminalSafeText(m.longReader.User.Name))
	header := "  [LONG MESSAGE] [" + m.longReader.Time.Local().Format("15:04") + "] " + readerName + " · Esc: close"
	closeText := "[X]"
	closeWidth := lipgloss.Width(closeText)
	headerWidth := max(0, contentWidth-closeWidth)
	header = truncateANSI(m.styles.accent.Render(header), headerWidth)
	header = padANSIToWidth(header, headerWidth, fill)
	rows := []string{header + m.styles.accent.Render(closeText) + railStyle.Render("^")}
	if height > 1 {
		// One blank row between the header and the body.
		rows = append(rows, padANSIToWidth("", contentWidth, fill)+railStyle.Render("│"))
	}
	visible := max(0, height-len(rows))
	thumbTop, thumbHeight, trackHeight := m.longReaderScrollbarThumb()
	for i := 0; i < visible; i++ {
		index := m.longReaderScroll+i
		line := ""
		if index < len(m.longReaderLines) {
			line = m.longReaderLines[index]
		}
		line = padANSIToWidth(fill.Render(" "+line), contentWidth, fill)
		rail := "│"
		switch {
		case i == visible-1:
			rail = "v"
		case m.maxLongReaderScroll() > 0 && i < trackHeight && i >= thumbTop && i < thumbTop+thumbHeight:
			rail = "█"
		case i >= trackHeight:
			rail = " "
		}
		rows = append(rows, line+railStyle.Render(rail))
	}
	return m.chatBorderStyle().Width(m.width).Height(m.messageHeight()).Render(strings.Join(rows, "\n"))
}
