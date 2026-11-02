package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const replyComposerPrefix = "^Reply: "

type messageBodyRange struct {
	Start int // inclusive column within the chat content row
	End   int // exclusive column within the chat content row
}

type messageLinkRange struct {
	Start    int
	End      int
	TargetID string
}

type messageRowRange struct {
	Start int
	End   int // exclusive
}

type messageNavigationHighlightTimeoutMsg struct {
	generation int
}

// messageBodyRanges returns one body-only hitbox for every rendered row of a
// message. Header rows, timestamps, indentation and unused row space are not
// interactive. The calculations mirror the wide and compact render layouts.
func (m model) messageBodyRanges(msg Packet, width int) []messageBodyRange {
	lines := m.renderPacketLines(msg, width)
	return m.messageBodyRangesForLineCount(msg, width, len(lines))
}

func (m model) messageBodyRangesForLineCount(msg Packet, width, lineCount int) []messageBodyRange {
	ranges := make([]messageBodyRange, lineCount)
	if msg.Type != "message" || msg.Text == "" || msg.XKeyscore > 0 || lineCount == 0 {
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
	replyBadge := m.renderReplyBadgeForWidth(msg, width)
	badgeWidth := lipgloss.Width(replyBadge)
	prefix := timestamp + bodyStyle.Render(" ") + nameStyle.Render(msg.User.Name)
	if replyBadge != "" {
		prefix += bodyStyle.Render(" ") + replyBadge
	}
	prefix += bodyStyle.Render(": ")
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
			end := start + lipgloss.Width(part)
			ranges[row] = clippedMessageBodyRange(start, end, contentWidth)
		}
		return ranges
	}

	// Compact layout: the username is on its own first row, and the timestamp
	// shares the continuation row with body text when a continuation exists.
	nameWidth := contentWidth - 2 - badgeWidth
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

	labelWidth := badgeWidth + lipgloss.Width(nameParts[0] + ": ")
	firstWidth := contentWidth - labelWidth
	wrapped := wrapTextVariable(msg.Text, firstWidth, contentWidth-stampWidth-1, contentWidth)
	if msg.Rainbow {
		wrapped = rainbowParts(wrapped, msg.RainbowPhase)
	}
	if len(wrapped) > 0 && wrapped[0] != "" {
		ranges[0] = clippedMessageBodyRange(labelWidth+1, labelWidth+1+lipgloss.Width(wrapped[0]), contentWidth)
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

// refreshMessageHover recalculates the body hover under the last pointer
// position after scrolling, resizing, or opening and closing a side panel.
func (m *model) restyleMessageRows(messageID string) bool {
	if messageID == "" {
		return true
	}
	if !m.renderCacheValid || m.renderCacheWidth != m.width {
		return false
	}
	rows, exists := m.messageRowsByID[messageID]
	if !exists {
		return true
	}
	target, exists := m.messageByID[messageID]
	if !exists || rows.Start < 0 || rows.End > len(m.renderedLines) || rows.End < rows.Start {
		return false
	}
	updated := m.renderPacketLines(target, m.width)
	if len(updated) != rows.End-rows.Start {
		return false
	}
	copy(m.renderedLines[rows.Start:rows.End], updated)
	return true
}

func (m *model) refreshMessageHover() {
	if !m.mousePositionKnown {
		return
	}
	hoveredID := ""
	if !m.usersOpen && !m.splashOpen {
		hoveredID = m.messageIDAt(m.lastMouseX, m.lastMouseY)
	}
	if hoveredID == m.hoveredMessageID {
		return
	}

	previousID := m.hoveredMessageID
	m.hoveredMessageID = hoveredID
	// Hover changes only foreground styling; they do not change wrapping or
	// hitboxes. Restyle the affected cached rows in place instead of laying
	// out the entire chat history on every pointer transition.
	if !m.restyleMessageRows(previousID) || !m.restyleMessageRows(hoveredID) {
		m.rebuildMessageCache()
	}
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
			messageRanges := m.messageBodyRangesForLineCount(packet, m.width, len(messageLines))
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
	if !m.hasMessageID(id) {
		return ""
	}
	return id
}


func (m model) messageLinkRanges(msg Packet, width int) [][]messageLinkRange {
	lines := m.renderPacketLines(msg, width)
	return m.messageLinkRangesForLineCount(msg, width, len(lines))
}

func (m model) messageLinkRangesForLineCount(msg Packet, width, lineCount int) [][]messageLinkRange {
	regions := make([][]messageLinkRange, lineCount)
	if len(regions) == 0 || msg.Type != "message" {
		return regions
	}

	if msg.ReplyTo != "" && m.hasMessageID(msg.ReplyTo) {
		badgeStart := 0
		if width > 48 {
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.User.Color)).Background(lipgloss.Color(m.cfg.Background)).Bold(true)
			timestamp := m.styles.time.Render("[" + msg.Time.Local().Format("15:04") + "]")
			badgeStart = lipgloss.Width(timestamp + " " + nameStyle.Render(msg.User.Name) + " ")
		}
		regions[0] = append(regions[0], messageLinkRange{Start: badgeStart, End: badgeStart+lipgloss.Width(strings.TrimSpace(m.renderReplyBadgeForWidth(msg, width))), TargetID: msg.ReplyTo})
	}
	replies := m.repliesFor(msg.ID)
	if len(replies) > 0 && msg.XKeyscore == 0 {
		base := m.renderPacketBaseLines(msg, width)
		row, column, contentWidth := len(base)-1, visibleChatTextWidth(base[len(base)-1]), chatContentWidth(width)
		for n, replyID := range replies {
			markerWidth := lipgloss.Width(fmt.Sprintf("[%d]", n+1))
			if column+1+markerWidth <= contentWidth {
				regions[row] = append(regions[row], messageLinkRange{Start:column+1,End:column+1+markerWidth,TargetID:replyID})
				column += 1+markerWidth
			} else {
				row++; column=0
				if row >= len(regions) { break }
				regions[row] = append(regions[row], messageLinkRange{Start:0,End:markerWidth,TargetID:replyID})
				column=markerWidth
			}
		}
	}
	return regions
}

// messageLinkAt detects clicks on [Replied] or an individual ^ reply marker.
func (m model) messageLinkAt(x, y int) string {
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

	links := m.renderedMessageLinks
	if !m.renderCacheValid || len(links) != len(lines) {
		links = make([][]messageLinkRange, 0, len(lines))
		for _, packet := range m.messages {
			messageLines := m.renderPacketLines(packet, m.width)
			messageLinks := m.messageLinkRangesForLineCount(packet, m.width, len(messageLines))
			for i := range messageLines {
				if i < len(messageLinks) {
					links = append(links, messageLinks[i])
				} else {
					links = append(links, nil)
				}
			}
		}
	}
	if row >= len(links) {
		return ""
	}
	localX := x - left
	for _, region := range links[row] {
		if localX >= region.Start && localX < region.End {
			return region.TargetID
		}
	}
	return ""
}

// navigateToMessage scrolls the target into the middle of the chat viewport
// and briefly colours its body so the destination is easy to spot.
func (m *model) navigateToMessage(id string) tea.Cmd {
	if id == "" {
		return nil
	}
	if !m.renderCacheValid || len(m.renderedMessageIDs) != len(m.renderMessageLines()) {
		m.rebuildMessageCache()
	}

	row := -1
	for i, messageID := range m.renderedMessageIDs {
		if messageID == id {
			row = i
			break
		}
	}
	if row < 0 {
		return nil
	}

	visibleHeight := max(1, m.messageHeight()-1)
	start := row - visibleHeight/2
	maxStart := max(0, len(m.renderedLines)-visibleHeight)
	if start < 0 {
		start = 0
	}
	if start > maxStart {
		start = maxStart
	}
	m.scroll = len(m.renderedLines) - visibleHeight - start
	m.clampScroll()

	m.highlightedMessageID = id
	m.navigationHighlightGeneration++
	generation := m.navigationHighlightGeneration
	m.rebuildMessageCache()
	return tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg {
		return messageNavigationHighlightTimeoutMsg{generation: generation}
	})
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
				m.clampScroll()
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
	m.clampScroll()
}
