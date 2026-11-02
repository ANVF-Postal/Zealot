package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m model) splashRows() []string {
	const logoWidth = 74
	logo := []string{
		"#########    #########    ..#####..    ###......    .#######.    #########",
		"......###    ###......    .###.###.    ###......    ###...###    ....###..",
		".....###.    ###......    ###...###    ###......    ###...###    ....###..",
		"....###..    ########.    #########    ###......    ###...###    ....###..",
		"..###....    ###......    ###...###    ###......    ###...###    ....###..",
		".###.....    ###......    ###...###    ###......    ###...###    ....###..",
		"#########    #########    ###...###    #########    .#######.    ....###..",
	}
	width := chatContentWidth(m.width)
	center := func(value string) string {
		value = truncateANSI(value, width)
		padding := max(0, (width-lipgloss.Width(value))/2)
		return strings.Repeat(" ", padding) + value
	}

	room := m.room
	if !m.roomExplicit {
		room = "default"
	}

	rows := []string{""}
	for _, line := range logo {
		if lipgloss.Width(line) != logoWidth {
			// Keep the layout predictable if the artwork is edited later.
			line = truncateANSI(line, logoWidth)
		}
		rows = append(rows, m.styles.accent.Render(center(line)))
	}
	rows = append(rows, "")
	rows = append(rows, m.styles.app.Render(center(appVersion)))
	rows = append(rows, "")
	rows = append(rows, m.styles.app.Render(center("Connected to: "+room)))
	historyStatus := "FALSE"
	if m.historyEnabled {
		historyStatus = "TRUE"
	}
	rows = append(rows, m.styles.app.Render(center("History Enabled: "+historyStatus)))
	rows = append(rows, m.styles.app.Render(center(fmt.Sprintf("Online users: %d", m.onlineCount))))
	return rows
}

func (m model) splashPanelHeight() int {
	return max(1, m.messageHeight())
}

func (m model) maxSplashScroll() int {
	return max(0, len(m.splashRows())-m.splashPanelHeight())
}

func (m *model) clampSplashScroll() {
	if m.splashScroll < 0 { m.splashScroll = 0 }
	if maxScroll := m.maxSplashScroll(); m.splashScroll > maxScroll { m.splashScroll = maxScroll }
}

func (m model) splashScrollbarThumb() (top, thumbHeight, trackHeight int) {
	visible := m.splashPanelHeight()
	trackHeight = visible - 2
	if trackHeight < 1 { trackHeight = 1 }
	total := len(m.splashRows())
	if total <= visible { return 0, trackHeight, trackHeight }
	thumbHeight = visible * trackHeight / total
	if thumbHeight < 1 { thumbHeight = 1 }
	if thumbHeight > trackHeight { thumbHeight = trackHeight }
	maxTop := trackHeight - thumbHeight
	if maxScroll := m.maxSplashScroll(); maxScroll > 0 {
		top = m.splashScroll * maxTop / maxScroll
	}
	return
}

func (m *model) setSplashScrollFromThumbTop(top int) {
	_, thumbHeight, trackHeight := m.splashScrollbarThumb()
	maxTop := trackHeight - thumbHeight
	if top < 0 { top = 0 }
	if top > maxTop { top = maxTop }
	if maxTop > 0 {
		m.splashScroll = top * m.maxSplashScroll() / maxTop
	} else {
		m.splashScroll = 0
	}
	m.clampSplashScroll()
}

func (m *model) setSplashScrollFromTrack(trackPosition int) {
	_, thumbHeight, trackHeight := m.splashScrollbarThumb()
	if trackPosition < 0 { trackPosition = 0 }
	if trackPosition >= trackHeight { trackPosition = trackHeight - 1 }
	m.setSplashScrollFromThumbTop(trackPosition - thumbHeight/2)
}

func (m model) splashScrollbarHoldCmd(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return splashScrollbarHoldMsg{generation: generation}
	})
}

// usersPanelHeight is the available inner height for the online roster.
func (m model) usersPanelHeight() int {
	// The roster heading takes one row in the existing chat viewport.
	return max(1, m.messageHeight()-1)
}

func (m model) userRows() []string {
	width := m.width - 12
	if width < 8 { width = 8 }
	rows := []string{}
	for _, user := range m.onlineUsers {
		nameWidth := width - 5
		if nameWidth < 1 { nameWidth = 1 }
		parts := wrapText(user.Name, nameWidth)
		if len(parts) == 0 { parts = []string{user.Name} }
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color(user.Color)).
			Background(lipgloss.Color(m.cfg.Background)).
			Bold(true)
		marker := ""
		switch {
		case m.userActivity[user.Name] > 0 && m.usersFrame%2 == 0:
			marker = " [!]"
		case user.Typing:
			frames := []string{"·  ", "·· ", "···", " ··", "  ·"}
			marker = " " + frames[m.usersFrame%len(frames)]
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(m.cfg.AccentColor)).
				Background(lipgloss.Color(m.cfg.Background)).
				Bold(true)
		}
		for i, part := range parts {
			text := part
			if i == len(parts)-1 {
				text += marker
			}
			rows = append(rows, "    "+style.Render(text))
		}
	}
	if len(rows) == 0 { rows = []string{m.styles.system.Render("  Nobody else is here yet.")} }
	return rows
}

func (m model) usersScrollbarThumb() (top, thumbHeight, trackHeight int) {
	visible := m.usersPanelHeight()
	// The heading row carries the top arrow. The final user row carries the
	// bottom arrow, leaving one track cell for each intervening user row.
	trackHeight = visible - 1
	if trackHeight < 1 {
		trackHeight = 1
	}
	total := len(m.userRows())
	if total <= visible {
		return 0, trackHeight, trackHeight
	}
	thumbHeight = visible * trackHeight / total
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	if thumbHeight > trackHeight {
		thumbHeight = trackHeight
	}
	maxTop := trackHeight - thumbHeight
	maxScroll := m.maxUsersScroll()
	if maxScroll > 0 {
		top = m.usersScroll * maxTop / maxScroll
	}
	return
}

func (m *model) setUsersScrollFromThumbTop(top int) {
	_, thumbHeight, trackHeight := m.usersScrollbarThumb()
	maxTop := trackHeight - thumbHeight
	if top < 0 { top = 0 }
	if top > maxTop { top = maxTop }
	if maxTop > 0 {
		m.usersScroll = top * m.maxUsersScroll() / maxTop
	} else {
		m.usersScroll = 0
	}
	m.clampUsersScroll()
}

func (m *model) setUsersScrollFromTrack(trackPosition int) {
	_, thumbHeight, trackHeight := m.usersScrollbarThumb()
	if trackPosition < 0 { trackPosition = 0 }
	if trackPosition >= trackHeight { trackPosition = trackHeight - 1 }
	m.setUsersScrollFromThumbTop(trackPosition - thumbHeight/2)
}

func (m model) usersScrollbarHoldCmd(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return usersScrollbarHoldMsg{generation: generation}
	})
}

func (m model) maxUsersScroll() int {
	maxScroll := len(m.userRows()) - m.usersPanelHeight()
	if maxScroll < 0 { return 0 }
	return maxScroll
}

func (m *model) clampUsersScroll() {
	if m.usersScroll < 0 { m.usersScroll = 0 }
	if m.usersScroll > m.maxUsersScroll() { m.usersScroll = m.maxUsersScroll() }
}


// -----------------------------------------------------------------------------
// UI layout
// -----------------------------------------------------------------------------

// statusBarVisible keeps the keyboard-shortcut row while the terminal is
// at least 15 rows tall. Below that, the chat and composer get the space.
func (m model) statusBarVisible() bool {
	return m.height >= 15
}

// resizeInput recalculates the textarea width and its available height.
// Static mode protects a three-row chat viewport before reducing the composer.
// Adaptive mode keeps two chat rows while allowing the composer to grow.
func (m *model) resizeInput() {
	baseHeight := m.cfg.InputHeight
	if baseHeight < 1 {
		baseHeight = 1
	}
	if baseHeight > 8 {
		baseHeight = 8
	}

	// In static mode, begin giving the chat pane more room as soon as the
	// terminal drops below 17 rows. Reduce the composer by one content row
	// for every terminal row below that threshold, down to its one-row floor.
	// This makes the configured height the starting height, not a hard minimum.
	if !m.cfg.AdaptiveHeight && baseHeight > 1 {
		rowsBelowThreshold := max(0, 17-m.height)
		baseHeight = max(1, baseHeight-rowsBelowThreshold)
	}

	minChatHeight := 3
	if m.cfg.AdaptiveHeight {
		minChatHeight = 2
	}

	// Header + borders around the chat and composer. The shortcut row is
	// removed only after the composer has been reduced to one content row.
	fixedChrome := 5
	if m.statusBarVisible() {
		fixedChrome++
	}
	layoutMax := m.height - fixedChrome - minChatHeight
	if layoutMax < 1 {
		layoutMax = 1
	}
	if baseHeight > layoutMax {
		baseHeight = layoutMax
	}

	if m.cfg.AdaptiveHeight {
		percent := m.cfg.HeightMaxPercent
		if percent < 1 {
			percent = 1
		}
		if percent > 100 {
			percent = 100
		}

		// The percentage applies to the full composer, including its borders.
		maxHeight := (m.height*percent)/100 - 2
		if maxHeight < baseHeight {
			maxHeight = baseHeight
		}
		if maxHeight > layoutMax {
			maxHeight = layoutMax
		}

		m.input.DynamicHeight = true
		m.input.MinHeight = baseHeight
		m.input.MaxHeight = maxHeight
		// MaxHeight controls the visible viewport. Keep the full message
		// editable up to Zealot's existing character limit.
		m.input.MaxContentHeight = maxMessage
	} else {
		m.input.DynamicHeight = false
		m.input.MinHeight = baseHeight
		m.input.MaxHeight = 0
		m.input.MaxContentHeight = 0
		m.input.SetHeight(baseHeight)
	}

	// Preserve the previous narrow-terminal width behavior, but still call
	// SetWidth so DynamicHeight recalculates after a vertical resize.
	width := m.input.Width()
	if m.width >= 20 {
		width = m.width - 6 // Side borders + padding + the two-column first-row prompt.
		if narrowBordersHidden(m.width) {
			width += 2
		}
	}
	minInputWidth := 10
	if m.replyTargetID != "" {
		width -= len([]rune(replyComposerPrefix))
		minInputWidth = 1
	}
	if width < minInputWidth {
		width = minInputWidth
	}
	m.input.SetWidth(width)
}

func (m model) messageHeight() int {
	// The chat pane takes what remains after the header, composer, borders,
	// and the keyboard-shortcut row when that row is visible.
	inputHeight := m.input.Height()
	if inputHeight < 1 {
		inputHeight = 1
	}
	fixedChrome := 5
	if m.statusBarVisible() {
		fixedChrome++
	}
	h := m.height - inputHeight - fixedChrome
	if h < 2 {
		return 2
	}
	return h
}

func (m *model) appendMessage(packet Packet) {
	oldScroll := m.scroll
	if m.width < 1 {
		m.messages = append(m.messages, packet)
		m.addMessageToIndexes(packet)
		m.renderCacheValid = false
		return
	}

	cacheValid := m.renderCacheValid && m.renderCacheWidth == m.width
	oldLineCount := 0
	if cacheValid {
		oldLineCount = len(m.renderedLines)
	} else if oldScroll > 0 {
		// Preserve the reader's historical position when the cache is stale.
		// At the live tail, no pre-append layout is needed.
		m.rebuildMessageCache()
		oldLineCount = len(m.renderedLines)
		cacheValid = true
	}
	m.messages = append(m.messages, packet)

	// Replies change both their own [Replied] badge and the original message's
	// numbered markers, so those updates need a complete cache rebuild.
	if (packet.Type == "message" && packet.ReplyTo != "") || !cacheValid {
		m.rebuildMessageCache()
		if oldScroll > 0 {
			m.scroll += len(m.renderedLines) - oldLineCount
		}
		m.clampScroll()
		return
	}

	m.addMessageToIndexes(packet)
	newLines := m.renderPacketLines(packet, m.width)
	bodyRanges := m.messageBodyRangesForLineCount(packet, m.width, len(newLines))
	linkRanges := m.messageLinkRangesForLineCount(packet, m.width, len(newLines))
	for i := range newLines {
		m.renderedMessageIDs = append(m.renderedMessageIDs, packet.ID)
		if i < len(bodyRanges) {
			m.renderedMessageBodyRanges = append(m.renderedMessageBodyRanges, bodyRanges[i])
		} else {
			m.renderedMessageBodyRanges = append(m.renderedMessageBodyRanges, messageBodyRange{})
		}
		if i < len(linkRanges) {
			m.renderedMessageLinks = append(m.renderedMessageLinks, linkRanges[i])
		} else {
			m.renderedMessageLinks = append(m.renderedMessageLinks, nil)
		}
	}
	if m.scroll > 0 {
		// Scroll is an offset from the bottom. Increase it by the number of
		// appended rows so the same historical rows remain on screen while
		// the user is reading back. At zero, continue following the live tail.
		m.scroll += len(newLines)
	}

	if m.renderCacheValid && m.renderCacheWidth == m.width {
		m.renderedLines = append(m.renderedLines, newLines...)
	} else {
		m.renderCacheValid = false
	}
	m.clampScroll()
}

func (m *model) rebuildMessageCache() {
	m.rebuildMessageIndexes()
	if m.width < 1 {
		m.renderedLines = nil
		m.renderedMessageIDs = nil
		m.renderedMessageBodyRanges = nil
		m.renderedMessageLinks = nil
		m.renderCacheValid = false
		return
	}

	lines := make([]string, 0, len(m.messages))
	messageIDs := make([]string, 0, len(m.messages))
	bodyRanges := make([]messageBodyRange, 0, len(m.messages))
	linkRanges := make([][]messageLinkRange, 0, len(m.messages))
	for _, message := range m.messages {
		messageLines := m.renderPacketLines(message, m.width)
		messageRanges := m.messageBodyRangesForLineCount(message, m.width, len(messageLines))
		messageLinks := m.messageLinkRangesForLineCount(message, m.width, len(messageLines))
		lines = append(lines, messageLines...)
		for i := range messageLines {
			messageIDs = append(messageIDs, message.ID)
			if i < len(messageRanges) {
				bodyRanges = append(bodyRanges, messageRanges[i])
			} else {
				bodyRanges = append(bodyRanges, messageBodyRange{})
			}
			if i < len(messageLinks) {
				linkRanges = append(linkRanges, messageLinks[i])
			} else {
				linkRanges = append(linkRanges, nil)
			}
		}
	}
	m.renderedLines = lines
	m.renderedMessageIDs = messageIDs
	m.renderedMessageBodyRanges = bodyRanges
	m.renderedMessageLinks = linkRanges
	m.renderCacheWidth = m.width
	m.renderCacheValid = true
}

func (m model) maxScroll() int {
	lines := m.renderMessageLines()

	// One line is permanently reserved for typing.
	visibleHeight := m.messageHeight() - 1

	if visibleHeight < 1 {
		visibleHeight = 1
	}

	max := len(lines) - visibleHeight

	if max < 0 {
		return 0
	}

	return max
}

func (m *model) clampScroll() {
	max := m.maxScroll()
	if m.scroll < 0 { m.scroll = 0 }
	if m.scroll > max { m.scroll = max }
	m.refreshMessageHover()
}

func (m model) scrollbarContains(x, y int) bool {
	return x == m.chatScrollbarX() && y >= 2 && y <= 1+m.messageHeight()
}

func (m model) scrollbarThumb() (top, thumbHeight, trackHeight int) {
	return m.scrollbarThumbForLineCount(len(m.renderMessageLines()))
}

func (m model) scrollbarThumbForLineCount(total int) (top, thumbHeight, trackHeight int) {
	trackHeight = m.messageHeight() - 2
	if trackHeight < 1 {
		trackHeight = 1
	}
	visible := m.messageHeight() - 1
	if visible < 1 {
		visible = 1
	}
	if total <= visible {
		return 0, trackHeight, trackHeight
	}

	thumbHeight = visible * trackHeight / total
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	if thumbHeight > trackHeight {
		thumbHeight = trackHeight
	}
	maxTop := trackHeight - thumbHeight
	maxScroll := total - visible
	if maxScroll > 0 {
		top = (maxScroll - m.scroll) * maxTop / maxScroll
	}
	return
}

func (m *model) setScrollFromTrack(trackPosition int) {
	_, thumbHeight, trackHeight := m.scrollbarThumb()
	m.setScrollFromThumbTop(trackPosition-thumbHeight/2)
	_ = trackHeight
}

func (m *model) setScrollFromThumbTop(top int) {
	_, thumbHeight, trackHeight := m.scrollbarThumb()
	maxTop := trackHeight-thumbHeight
	if top < 0 { top = 0 }
	if top > maxTop { top = maxTop }
	if maxTop > 0 {
		m.scroll = m.maxScroll() - top*m.maxScroll()/maxTop
	} else {
		m.scroll = 0
	}
	m.clampScroll()
}

func (m model) scrollbarHoldCmd(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return scrollbarHoldMsg{generation: generation}
	})
}
