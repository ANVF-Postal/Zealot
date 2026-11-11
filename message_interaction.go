package main

import (
	"fmt"
	"sort"
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
	msg = m.displayedMessage(msg)
	ranges := make([]messageBodyRange, lineCount)
	if (msg.Type != "message" && msg.Type != "whisper_result") || msg.Text == "" || msg.XKeyscore > 0 || lineCount == 0 {
		return ranges
	}

	contentWidth := chatContentWidth(width)
	if msg.Type == "whisper_result" {
		// Use the same contextual header geometry as the renderer; all
		// coordinates start at the first chat cell, with no special padding.
		before, after, compact := m.whisperHeaderParts(msg, width)
		headerWidth := lipgloss.Width(before+after) +
			lipgloss.Width(m.renderReplyBadgeForWidth(msg, width))
		bodyWidth := lipgloss.Width(terminalSafeText(msg.Text))
		limit := max(1, contentWidth)
		for row := range ranges {
			if compact && row == len(ranges)-1 {
				break // muted timestamp occupies a separate final row
			}
			start := max(0, headerWidth-row*limit)
			end := min(limit, headerWidth+bodyWidth-row*limit)
			if start < limit && end > start {
				ranges[row] = clippedMessageBodyRange(start, end, contentWidth)
			}
		}
		return ranges
	}
	if contentWidth < 1 {
		return ranges
	}

	bg := m.messageBackground(msg)
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
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
		prefix += replyBadge
	}
	prefix += bodyStyle.Render(": ")
	prefixWidth := lipgloss.Width(prefix)
	if width > 48 && prefixWidth >= contentWidth {
		badgeWidth = lipgloss.Width(m.renderReplyBadgeForWidth(msg, 48))
	}

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

// cachedBodyRangesForMessage calculates hover geometry only when a message is
// actually under the pointer. The result is reused until that message is
// rerendered or the terminal width changes.
func (m model) cachedBodyRangesForMessage(messageID string, lineCount int) []messageBodyRange {
	if m.renderCacheValid {
		if ranges, ok := m.messageBodyRangesByID[messageID]; ok {
			return ranges
		}
		if rows, ok := m.messageRowsByID[messageID]; ok {
			lineCount = rows.End - rows.Start
		}
	}
	packet, exists := m.messageByID[messageID]
	if !exists {
		for _, candidate := range m.messages {
			if candidate.ID == messageID && (candidate.Type == "message" || candidate.Type == "whisper_result") {
				packet, exists = candidate, true
				break
			}
		}
	}
	if !exists {
		return nil
	}
	if lineCount < 1 {
		lineCount = len(m.renderPacketLines(packet, m.width))
	}
	ranges := m.messageBodyRangesForLineCount(packet, m.width, lineCount)
	if m.renderCacheValid && m.messageBodyRangesByID != nil {
		m.messageBodyRangesByID[messageID] = ranges
	}
	return ranges
}

// cachedLinkRangesForMessage calculates reply-link geometry on demand. Most
// messages are never clicked, so resize work no longer builds their hitboxes.
func (m model) cachedLinkRangesForMessage(messageID string, lineCount int) [][]messageLinkRange {
	if m.renderCacheValid {
		if ranges, ok := m.messageLinkRangesByID[messageID]; ok {
			return ranges
		}
		if rows, ok := m.messageRowsByID[messageID]; ok {
			lineCount = rows.End - rows.Start
		}
	}
	packet, exists := m.messageByID[messageID]
	if !exists {
		for _, candidate := range m.messages {
			if candidate.ID == messageID && (candidate.Type == "message" || candidate.Type == "whisper_result") {
				packet, exists = candidate, true
				break
			}
		}
	}
	if !exists {
		return nil
	}
	if lineCount < 1 {
		lineCount = len(m.renderPacketLines(packet, m.width))
	}
	ranges := m.messageLinkRangesForLineCount(packet, m.width, lineCount)
	if m.renderCacheValid && m.messageLinkRangesByID != nil {
		m.messageLinkRangesByID[messageID] = ranges
	}
	return ranges
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
	if !m.renderWindowValid || rows.Start < m.renderWindowStart || rows.End > m.renderWindowEnd {
		return true // Off-screen messages have no styled rows to restyle.
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
	pollID, pollChoice := "", 0
	if !m.usersOpen && !m.splashOpen {
		hoveredID = m.messageIDAt(m.lastMouseX, m.lastMouseY)
		pollID, pollChoice = m.pollOptionAt(m.lastMouseX, m.lastMouseY)
	}
	if hoveredID == m.hoveredMessageID && pollID == m.pollHoveredID && pollChoice == m.pollHoveredOption {
		return
	}

	previousID, previousPoll := m.hoveredMessageID, m.pollHoveredID
	m.hoveredMessageID = hoveredID
	m.pollHoveredID, m.pollHoveredOption = pollID, pollChoice
	// Reuse the same targeted row restyling for chat and poll options.
	if !m.restyleMessageRows(previousID) || !m.restyleMessageRows(hoveredID) ||
		!m.restyleMessageRows(previousPoll) || !m.restyleMessageRows(pollID) {
		m.rebuildMessageCache()
	}
}


 // messageAtRow maps an absolute history row back to its packet using the
 // lightweight per-message row index. Off-screen messages need no string rows.
func (m model) messageAtRow(row int) (Packet, messageRowRange, bool) {
	if row < 0 {
		return Packet{}, messageRowRange{}, false
	}
	if m.renderCacheValid && len(m.messageRowRanges) == len(m.messages) {
		index := sort.Search(len(m.messageRowRanges), func(i int) bool {
			return m.messageRowRanges[i].End > row
		})
		if index < len(m.messageRowRanges) {
			rows := m.messageRowRanges[index]
			if row >= rows.Start && row < rows.End {
				return m.messages[index], rows, true
			}
		}
		return Packet{}, messageRowRange{}, false
	}

	start := 0
	for _, packet := range m.messages {
		end := start + max(1, m.renderPacketLineCount(packet, m.width))
		rows := messageRowRange{Start: start, End: end}
		if row >= start && row < end {
			return packet, rows, true
		}
		start = end
	}
	return Packet{}, messageRowRange{}, false
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

	packet, messageRows, ok := m.messageAtRow(row)
	if !ok || packet.ID == "" {
		return ""
	}
	id := packet.ID
	bodyRanges := m.cachedBodyRangesForMessage(id, messageRows.End-messageRows.Start)
	relativeRow := row - messageRows.Start
	if relativeRow >= len(bodyRanges) {
		return ""
	}
	localX := x - left
	body := bodyRanges[relativeRow]
	if localX < body.Start || localX >= body.End {
		return ""
	}
	if !m.hasMessageID(id) {
		return ""
	}
	return id
}


// messageUsernameAt returns the username only when a click lands on the name
// portion of a message header. Both wide and compact layouts use the same
// wrapping calculations as their renderers.
func (m model) messageUsernameAt(x, y int) string {
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
	packet, messageRows, ok := m.messageAtRow(row)
	if !ok || packet.Type != "message" {
		return ""
	}
	relativeRow := row - messageRows.Start
	localX := x - left
	contentWidth := chatContentWidth(m.width)
	timeWidth := lipgloss.Width("[" + packet.Time.Local().Format("15:04") + "]")
	if m.width > 48 {
		badge := m.renderReplyBadgeForWidth(packet, m.width)
		prefixExtra := 2 // colon and body gap for ordinary messages
		if packet.XKeyscore > 0 {
			prefixExtra = 1 // XKeyscore headers end directly after the colon
		}
		prefixWidth := timeWidth + 1 + lipgloss.Width(packet.User.Name) + lipgloss.Width(badge) + prefixExtra
		if prefixWidth < contentWidth {
			nameStart := timeWidth + 1
			nameEnd := nameStart + lipgloss.Width(packet.User.Name)
			if relativeRow == 0 && localX >= nameStart && localX < nameEnd {
				return packet.User.Name
			}
			return ""
		}
	}

	badge := m.renderReplyBadgeForWidth(packet, 48)
	nameWidth := contentWidth - 2 - lipgloss.Width(badge)
	if nameWidth < 1 {
		nameWidth = 1
	}
	nameParts := wrapText(packet.User.Name, nameWidth)
	if len(nameParts) == 0 {
		nameParts = []string{""}
	}
	if relativeRow >= 0 && relativeRow < len(nameParts) && localX >= 0 && localX < lipgloss.Width(nameParts[relativeRow]) {
		return packet.User.Name
	}
	return ""
}

func (m model) messageLinkRanges(msg Packet, width int) [][]messageLinkRange {
	lines := m.renderPacketLines(msg, width)
	return m.messageLinkRangesForLineCount(msg, width, len(lines))
}

func (m model) messageLinkRangesForLineCount(msg Packet, width, lineCount int) [][]messageLinkRange {
	regions := make([][]messageLinkRange, lineCount)
	if len(regions) == 0 || (msg.Type != "message" && msg.Type != "whisper_result") {
		return regions
	}

	if msg.ReplyTo != "" && m.hasMessageID(msg.ReplyTo) {
		if msg.Type == "whisper_result" {
			// Reuse the contextual header's pre-badge text rather than
			// guessing at the sender/recipient layout.
			before, _, _ := m.whisperHeaderParts(msg, width)
			start := lipgloss.Width(before)
			limit := max(1, chatContentWidth(width))
			row, column := start/limit, start%limit
			badgeWidth := lipgloss.Width(replyBadgeLabelForWidth(width))
			if row < len(regions) && column+badgeWidth <= limit {
				regions[row] = append(regions[row], messageLinkRange{
					Start: column, End: column+badgeWidth, TargetID: msg.ReplyTo,
				})
			}
		} else {
		badgeRow, badgeStart := 0, 0
		widthForBadge := width
		badge := m.renderReplyBadgeForWidth(msg, width)
		compact := width <= 48
		if width > 48 {
			bg := m.messageBackground(msg)
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.User.Color)).Background(bg).Bold(true)
			bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
			timestamp := m.styles.time.Render("[" + msg.Time.Local().Format("15:04") + "]")
			widePrefix := timestamp + bodyStyle.Render(" ") + nameStyle.Render(msg.User.Name) + badge + bodyStyle.Render(": ")
			if lipgloss.Width(widePrefix) < chatContentWidth(width) {
				badgeStart = lipgloss.Width(timestamp + " " + nameStyle.Render(msg.User.Name))
			} else {
				compact = true
				widthForBadge = 48
				badge = m.renderReplyBadgeForWidth(msg, widthForBadge)
			}
		}
		if compact {
			contentWidth := chatContentWidth(width)
			nameWidth := contentWidth - 2 - lipgloss.Width(badge)
			if nameWidth < 1 { nameWidth = 1 }
			nameParts := wrapText(msg.User.Name, nameWidth)
			if len(nameParts) == 0 { nameParts = []string{""} }
			badgeRow = len(nameParts)-1
			badgeStart = lipgloss.Width(nameParts[len(nameParts)-1])
		}
		if badgeRow < len(regions) {
			regions[badgeRow] = append(regions[badgeRow], messageLinkRange{Start: badgeStart, End: badgeStart + lipgloss.Width(replyBadgeLabelForWidth(widthForBadge)), TargetID: msg.ReplyTo})
		}
		}
	}
	replies := m.repliesFor(msg.ID)
	if len(replies) > 0 && msg.XKeyscore == 0 {
		base := m.renderPacketBaseLines(msg, width)
		row := len(base) - 1
		// In compact layout, a short body can fit beside the username while
		// the timestamp is rendered on a separate final row. Markers are
		// appended to the body row in that case, matching renderPacketLines.
		if width <= 48 && row > 0 {
			plainLast := strings.TrimSpace(stripANSISequences(base[row]))
			if plainLast == "["+msg.Time.Local().Format("15:04")+"]" {
				row--
			}
		}
		trimmed := trimANSIEndPadding(base[row])
		column, contentWidth := visibleChatTextWidth(trimmed), chatContentWidth(width)
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

// messageLinkAt detects clicks on [Re] or an individual numbered reply marker.
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

	packet, messageRows, ok := m.messageAtRow(row)
	if !ok || packet.ID == "" {
		return ""
	}
	id := packet.ID
	links := m.cachedLinkRangesForMessage(id, messageRows.End-messageRows.Start)
	relativeRow := row - messageRows.Start
	if relativeRow >= len(links) {
		return ""
	}
	localX := x - left
	for _, region := range links[relativeRow] {
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
	rows, exists := m.messageRowsByID[id]
	if !exists {
		return nil
	}
	row := rows.Start
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
	m.restyleMessageRows(id)
	return tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg {
		return messageNavigationHighlightTimeoutMsg{generation: generation}
	})
}

func (m *model) beginReply(id string) {
	if id == "" {
		return
	}
	for _, packet := range m.messages {
		if packet.ID == id && (packet.Type == "message" || packet.Type == "whisper_result") {
			if packet.Spoiler && !m.revealedSpoilers[id] {
				return
			}
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
