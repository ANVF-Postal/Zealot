package main

import (
	"strings"
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
	m.prepareLongReader()
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

func (m model) renderLongReaderPanel() string {
	bg := lipgloss.Color(m.cfg.Theme.Background)
	fill := lipgloss.NewStyle().Background(bg)
	width := max(1, chatInnerWidth(m.width))
	contentWidth := max(1, width-1)
	height := max(1, m.messageHeight())
	readerName := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(terminalSafeText(m.longReader.User.Name))
	header := m.styles.accent.Render("  [LONG MESSAGE] [" + m.longReader.Time.Local().Format("15:04") + "] " + readerName + " · Esc: close")
	header = padANSIToWidth(header, contentWidth, fill)
	rows := []string{header + m.styles.system.Render("^")}
	if height > 1 {
		// Keep a permanent empty row between the header and the body.
		rows = append(rows, padANSIToWidth("", contentWidth, fill)+m.styles.system.Render("│"))
	}
	visible := max(0, height-len(rows))
	for i := 0; i < visible; i++ {
		index := m.longReaderScroll+i
		line := ""
		if index < len(m.longReaderLines) {
			line = m.longReaderLines[index]
		}
		line = padANSIToWidth(fill.Render(" "+line), contentWidth, fill)
		rail := "│"
		if i == visible-1 {
			rail = "v"
		}
		rows = append(rows, line+m.styles.system.Render(rail))
	}
	return m.chatBorderStyle().Width(m.width).Height(m.messageHeight()).Render(strings.Join(rows, "\n"))
}
