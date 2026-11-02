package main

import "charm.land/lipgloss/v2"

const replyComposerPrefix = "^Reply: "

type messageBodyRange struct {
	Start int // inclusive column within the chat content row
	End   int // exclusive column within the chat content row
}

// messageBodyRanges returns one body-only hitbox for every rendered row of a
// message. Header rows, timestamps, indentation and unused row space are not
// interactive. The calculations mirror the wide and compact render layouts.
func (m model) messageBodyRanges(msg Packet, width int) []messageBodyRange {
	lines := m.renderPacketLines(msg, width)
	ranges := make([]messageBodyRange, len(lines))
	if msg.Type != "message" || msg.Text == "" || msg.XKeyscore > 0 || len(lines) == 0 {
		return ranges
	}

	contentWidth := chatContentWidth(width)
	if contentWidth < 1 {
		return ranges
	}

	bg := lipgloss.Color(m.cfg.Background)
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.TextColor)).Background(bg)
	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(msg.User.Color)).
		Background(bg).
		Bold(true)
	timeText := msg.Time.Local().Format("15:04")
	timestamp := m.styles.time.Render("[" + timeText + "]")
	prefix := timestamp + bodyStyle.Render(" ") + nameStyle.Render(msg.User.Name) + bodyStyle.Render(": ")
	prefixWidth := lipgloss.Width(prefix)

	// Standard wide layout: timestamp and username precede the body. Wrapped
	// body rows retain the same indentation as the first row.
	if width > 48 && prefixWidth < contentWidth {
		wrapped := wrapText(msg.Text, contentWidth-prefixWidth)
		if len(wrapped) == 0 {
			return ranges
		}
		for row, part := range wrapped {
			if row >= len(ranges) {
				break
			}
			start := prefixWidth
			if row > 0 {
				start = prefixWidth
			}
			end := start + lipgloss.Width(part)
			ranges[row] = clippedMessageBodyRange(start, end, contentWidth)
		}
		return ranges
	}

	// Compact layout: the username is on its own first row, and the timestamp
	// shares the continuation row with body text when a continuation exists.
	nameWidth := contentWidth - 2
	if nameWidth < 1 {
		nameWidth = 1
	}
	nameParts := wrapText(msg.User.Name, nameWidth)
	if len(nameParts) == 0 {
		nameParts = []string{""}
	}
	stamp := "[" + timeText + "]"
	stampWidth := lipgloss.Width(stamp)

	if len(nameParts) > 1 {
		wrapped := wrapTextVariable(msg.Text, 0, contentWidth-stampWidth-1, contentWidth)
		if msg.Rainbow {
			wrapped = rainbowParts(wrapped, msg.RainbowPhase)
		}
		bodyRow := len(nameParts)
		if len(wrapped) >= 2 && bodyRow < len(ranges) {
			ranges[bodyRow] = clippedMessageBodyRange(stampWidth+1, stampWidth+1+lipgloss.Width(wrapped[1]), contentWidth)
		}
		for i := 2; i < len(wrapped) && bodyRow+i-1 < len(ranges); i++ {
			ranges[bodyRow+i-1] = clippedMessageBodyRange(0, lipgloss.Width(wrapped[i]), contentWidth)
		}
		return ranges
	}

	labelWidth := lipgloss.Width(nameParts[0] + ": ")
	firstWidth := contentWidth - labelWidth
	wrapped := wrapTextVariable(msg.Text, firstWidth, contentWidth-stampWidth-1, contentWidth)
	if msg.Rainbow {
		wrapped = rainbowParts(wrapped, msg.RainbowPhase)
	}
	if len(wrapped) > 0 && wrapped[0] != "" {
		ranges[0] = clippedMessageBodyRange(labelWidth, labelWidth+lipgloss.Width(wrapped[0]), contentWidth)
	}
	if len(wrapped) >= 2 && len(ranges) > 1 {
		ranges[1] = clippedMessageBodyRange(stampWidth+1, stampWidth+1+lipgloss.Width(wrapped[1]), contentWidth)
	}
	for i := 2; i < len(wrapped) && i < len(ranges); i++ {
		ranges[i] = clippedMessageBodyRange(0, lipgloss.Width(wrapped[i]), contentWidth)
	}
	return ranges
}

func clippedMessageBodyRange(start, end, width int) messageBodyRange {
	if start < 0 {
		start = 0
	}
	if end > width {
		end = width
	}
	if end <= start {
		return messageBodyRange{}
	}
	return messageBodyRange{Start: start, End: end}
}

// messageIDAt returns the ID only when the pointer is over actual message body
// text. Username, timestamp, wrapping indentation and the scrollbar are inert.
func (m model) messageIDAt(x, y int) string {
	if m.usersOpen || m.splashOpen || m.width < 1 {
		return ""
	}
	left := 0
	if !narrowBordersHidden(m.width) {
		left = 1
	}
	if x < left || x >= m.chatScrollbarX() {
		return ""
	}

	chatHeight := max(1, m.messageHeight()-1)
	if y < 2 || y >= 2+chatHeight {
		return ""
	}

	lines := m.renderMessageLines()
	start := len(lines) - chatHeight - m.scroll
	if start < 0 {
		start = 0
	}
	row := start + y - 2
	if row < 0 || row >= len(lines) {
		return ""
	}

	ids := m.renderedMessageIDs
	ranges := m.renderedMessageBodyRanges
	if !m.renderCacheValid || len(ids) != len(lines) || len(ranges) != len(lines) {
		ids = make([]string, 0, len(lines))
		ranges = make([]messageBodyRange, 0, len(lines))
		for _, packet := range m.messages {
			messageLines := m.renderPacketLines(packet, m.width)
			messageRanges := m.messageBodyRanges(packet, m.width)
			for i := range messageLines {
				ids = append(ids, packet.ID)
				if i < len(messageRanges) {
					ranges = append(ranges, messageRanges[i])
				} else {
					ranges = append(ranges, messageBodyRange{})
				}
			}
		}
	}
	if row >= len(ids) || row >= len(ranges) || ids[row] == "" {
		return ""
	}

	localX := x - left
	body := ranges[row]
	if localX < body.Start || localX >= body.End {
		return ""
	}
	id := ids[row]
	for _, packet := range m.messages {
		if packet.ID == id && packet.Type == "message" {
			return id
		}
	}
	return ""
}

func (m *model) beginReply(id string) {
	if id == "" {
		return
	}
	for _, packet := range m.messages {
		if packet.ID == id && packet.Type == "message" {
			if m.replyTargetID != id {
				m.replyTargetID = id
				m.input.EndSelection()
				m.inputSelecting = false
				m.resizeInput()
			}
			return
		}
	}
}

func (m *model) cancelReply() {
	if m.replyTargetID == "" {
		return
	}
	m.replyTargetID = ""
	m.input.EndSelection()
	m.inputSelecting = false
	m.resizeInput()
}
