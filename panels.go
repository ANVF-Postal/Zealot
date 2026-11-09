package main

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// scaleScrollbarPosition maps one integer scrollbar range to another using
// nearest-integer rounding. Using truncation in both directions caused the
// one-cell thumb to render one track row away from the position the pointer
// selected, making the chat jump while the thumb appeared not to move.
func scaleScrollbarPosition(position, sourceMax, destinationMax int) int {
	if sourceMax <= 0 || destinationMax <= 0 {
		return 0
	}
	position = min(max(position, 0), sourceMax)
	numerator := int64(position) * int64(destinationMax)
	return int((numerator + int64(sourceMax)/2) / int64(sourceMax))
}

var splashLogos = [][]string{
	{
		"#########    #########    ..#####..    ###......    .#######.    #########",
		"......###    ###......    .###.###.    ###......    ###...###    ....###..",
		".....###.    ###......    ###...###    ###......    ###...###    ....###..",
		"....###..    ########.    #########    ###......    ###...###    ....###..",
		"..###....    ###......    ###...###    ###......    ###...###    ....###..",
		".###.....    ###......    ###...###    ###......    ###...###    ....###..",
		"#########    #########    ###...###    #########    .#######.    ....###..",
	},
	{
		"$$$$\\                    $\\            $\\     ",
		"\\____$  |                   $ |           $ |    ",
		"    $  / $$$\\   $$$\\  $ | $$$\\ $$$\\   ",
		"   $  / $  __$\\  \\____$\\ $ |$  __$\\\\_$  _|  ",
		"  $  /  $$$$ | $$$$ |$ |$ /  $ | $ |    ",
		" $  /   $   ____|$  __$ |$ |$ |  $ | $ |$\\ ",
		"$$$$\\\\$$$$\\ \\$$$$ |$ |\\$$$  | \\$$  |",
		"\\________|\\_______| \\_______|\\__| \\______/   \\____/ ",
	},
}

// cycleSplashLogo chooses a different logo each time the splash opens or
// its artwork is clicked; additional designs can be added to splashLogos.
func (m *model) cycleSplashLogo() {
	if len(splashLogos) < 2 {
		m.splashLogoIndex = 0
		return
	}
	m.splashLogoIndex = (m.splashLogoIndex + 1 + rand.IntN(len(splashLogos)-1)) % len(splashLogos)
	m.splashScroll = 0
}

func (m model) activeSplashLogo() []string {
	return splashLogos[min(max(0,m.splashLogoIndex),len(splashLogos)-1)]
}

// splashLogoContains follows the same centered, truncated rows shown on
// screen, accounting for splash scrolling and the chat content gutter.
func (m model) splashLogoContains(x, y int) bool {
	if !m.splashOpen || m.themesOpen {
		return false
	}
	width := chatContentWidth(m.width)
	for i, line := range m.activeSplashLogo() {
		rowY := 3+i-m.splashScroll // first splash row is blank
		if y != rowY || y < 2 || y >= 2+m.splashPanelHeight() {
			continue
		}
		visible := truncateANSI(line,width)
		startX := 1+max(0,(width-lipgloss.Width(visible))/2)
		return x >= startX && x < startX+lipgloss.Width(visible)
	}
	return false
}

func (m model) splashRows() []string {
	logo := m.activeSplashLogo()
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
		// Each design is kept at its original width, then centered or
		// truncated to fit the current terminal.
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
	rows = append(rows, "")
	rows = append(rows, m.styles.accent.Render(center("[THEMES]")))
	return rows
}

// splashThemesButtonContains reports whether a click lands on the centered
// [THEMES] action, accounting for the splash's vertical scroll offset.
func (m model) splashThemesButtonContains(x, y int) bool {
	button := "[THEMES]"
	row := len(m.splashRows()) - 1
	screenY := 2 + row - m.splashScroll // chat pane's top border and first content row
	startX := 1 + max(0, (chatContentWidth(m.width)-lipgloss.Width(button))/2)
	return y == screenY && x >= startX && x < startX+lipgloss.Width(button)
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
	return m.splashScrollbarThumbForRowCount(len(m.splashRows()))
}

func (m model) splashScrollbarThumbForRowCount(total int) (top, thumbHeight, trackHeight int) {
	visible := m.splashPanelHeight()
	trackHeight = visible - 2
	if trackHeight < 1 { trackHeight = 1 }
	if total <= visible { return 0, trackHeight, trackHeight }
	thumbHeight = visible * trackHeight / total
	if thumbHeight < 1 { thumbHeight = 1 }
	if thumbHeight > trackHeight { thumbHeight = trackHeight }
	maxTop := trackHeight - thumbHeight
	if maxScroll := total - visible; maxScroll > 0 {
		top = scaleScrollbarPosition(m.splashScroll, maxScroll, maxTop)
	}
	return
}

func (m *model) setSplashScrollFromThumbTop(top int) {
	_, thumbHeight, trackHeight := m.splashScrollbarThumb()
	maxTop := trackHeight - thumbHeight
	if top < 0 { top = 0 }
	if top > maxTop { top = maxTop }
	if maxTop > 0 {
		m.splashScroll = scaleScrollbarPosition(top, maxTop, m.maxSplashScroll())
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

// userRowCount calculates the roster height without formatting every user.
// Scroll clamping and scrollbar geometry need only this count, not rendered rows.
func (m model) userRowCount() int {
	if len(m.onlineUsers) == 0 {
		return 1
	}
	width := m.width - 12
	if width < 8 {
		width = 8
	}
	nameWidth := max(1, width-5)
	total := 0
	for _, user := range m.onlineUsers {
		parts := wrapText(user.Name, nameWidth)
		if len(parts) == 0 {
			total++
		} else {
			total += len(parts)
		}
	}
	return total
}

// userRows remains available for callers that need the complete rendered list.
func (m model) userRows() []string {
	return m.userRowsRange(0, m.userRowCount())
}

// userRowsRange formats only the requested visible rows. A large room roster
// should not require styling and allocating off-screen users on every redraw.
func (m model) userRowsRange(start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return nil
	}
	if len(m.onlineUsers) == 0 {
		if start == 0 {
			return []string{m.styles.system.Render("  Nobody else is here yet.")}
		}
		return nil
	}

	width := m.width - 12
	if width < 8 {
		width = 8
	}
	nameWidth := max(1, width-5)
	rows := make([]string, 0, end-start)
	rowIndex := 0
	for _, user := range m.onlineUsers {
		parts := wrapText(user.Name, nameWidth)
		if len(parts) == 0 {
			parts = []string{user.Name}
		}
		rowEnd := rowIndex + len(parts)
		if rowEnd > start && rowIndex < end {
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color(user.Color)).
				Background(lipgloss.Color(m.cfg.Theme.Background)).
				Bold(true)
			marker := ""
			switch {
			case m.userActivity[user.Name] > 0 && m.usersFrame%2 == 0:
				marker = " [!]"
			case user.Typing:
				frames := []string{"·  ", "·· ", "···", " ··", "  ·"}
				marker = " " + frames[m.usersFrame%len(frames)]
				style = lipgloss.NewStyle().
					Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
					Background(lipgloss.Color(m.cfg.Theme.Background)).
					Bold(true)
			}
			for i, part := range parts {
				if rowIndex >= start && rowIndex < end {
					text := part
					if i == len(parts)-1 {
						text += marker
					}
					rows = append(rows, "    "+style.Render(text))
				}
				rowIndex++
				if rowIndex >= end {
					return rows
				}
			}
		} else {
			rowIndex = rowEnd
			if rowIndex >= end {
				break
			}
		}
	}
	return rows
}

// userNameAt maps a click in the online roster back to the username rendered
// on that row. Wrapped usernames remain clickable across all of their rows.
func (m model) userNameAt(x, y int) string {
	if !m.usersOpen || y < 3 || x < 0 || x >= m.chatScrollbarX() {
		return ""
	}
	left := 0
	if !narrowBordersHidden(m.width) {
		left = 1
	}
	localX := x - left
	if localX < 4 {
		return ""
	}
	width := m.width - 12
	if width < 8 {
		width = 8
	}
	nameWidth := max(1, width-5)
	targetRow := m.usersScroll + y - 3
	row := 0
	for _, user := range m.onlineUsers {
		parts := wrapText(user.Name, nameWidth)
		if len(parts) == 0 {
			parts = []string{user.Name}
		}
		for _, part := range parts {
			partWidth := lipgloss.Width(part)
			if row == targetRow && localX >= 4 && localX < 4+partWidth {
				return user.Name
			}
			row++
		}
		if row > targetRow {
			break
		}
	}
	return ""
}

// usersAnimationActive prevents a permanently scheduled redraw loop when the
// roster is open but nobody is typing and there is no recent activity marker.
func (m model) usersAnimationActive() bool {
	if len(m.userActivity) > 0 {
		return true
	}
	if !m.usersOpen {
		return false
	}
	for _, user := range m.onlineUsers {
		if user.Typing {
			return true
		}
	}
	return false
}

func (m *model) startUsersAnimation() tea.Cmd {
	if m.usersAnimationRunning || !m.usersAnimationActive() {
		return nil
	}
	m.usersAnimationRunning = true
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return usersAnimationMsg{}
	})
}

func (m model) usersScrollbarThumb() (top, thumbHeight, trackHeight int) {
	return m.usersScrollbarThumbForRowCount(m.userRowCount())
}

func (m model) usersScrollbarThumbForRowCount(total int) (top, thumbHeight, trackHeight int) {
	visible := m.usersPanelHeight()
	// The heading row carries the top arrow. The final user row carries the
	// bottom arrow, leaving one track cell for each intervening user row.
	trackHeight = visible - 1
	if trackHeight < 1 {
		trackHeight = 1
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
	maxScroll := max(0, total-visible)
	if maxScroll > 0 {
		top = scaleScrollbarPosition(m.usersScroll, maxScroll, maxTop)
	}
	return
}

func (m *model) setUsersScrollFromThumbTop(top int) {
	_, thumbHeight, trackHeight := m.usersScrollbarThumb()
	maxTop := trackHeight - thumbHeight
	if top < 0 { top = 0 }
	if top > maxTop { top = maxTop }
	if maxTop > 0 {
		m.usersScroll = scaleScrollbarPosition(top, maxTop, m.maxUsersScroll())
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
	maxScroll := m.userRowCount() - m.usersPanelHeight()
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
	m.appendMessages([]Packet{packet})
}

// appendMessages updates indexes and cached rows for a group of packets while
// issuing only one scroll/hover refresh. During history replay this avoids
// repeating the model-level work for every decoded packet.
func (m *model) appendMessages(packets []Packet) {
	if len(packets) == 0 {
		return
	}

	oldScroll := m.scroll
	cacheValid := m.renderCacheValid && m.renderCacheWidth == m.width
	oldLineCount := 0
	if cacheValid {
		oldLineCount = len(m.renderedLines)
	} else if oldScroll > 0 && m.width > 0 {
		// Preserve the reader's position if a cache rebuild is required before
		// applying the batch.
		m.rebuildMessageCache()
		oldLineCount = len(m.renderedLines)
		cacheValid = true
	}

	m.messages = append(m.messages, packets...)
	if m.messageByID == nil || m.replyIDsByMessageID == nil {
		m.rebuildMessageIndexes()
	} else {
		for _, packet := range packets {
			m.addMessageToIndexes(packet)
		}
	}

	if m.width < 1 {
		m.renderCacheValid = false
		return
	}

	if !cacheValid {
		m.rebuildMessageCache()
		if oldScroll > 0 {
			m.scroll += len(m.renderedLines) - oldLineCount
		}
		m.clampScroll()
		return
	}

	// Existing cached parents need their reply markers refreshed. A parent
	// that is part of this same batch will be rendered with its complete reply
	// list below, so only messages already present in the cache are replaced.
	parents := make(map[string]struct{})
	for _, packet := range packets {
		if packet.Type != "message" || packet.ReplyTo == "" {
			continue
		}
		if _, cached := m.messageRowsByID[packet.ReplyTo]; cached {
			parents[packet.ReplyTo] = struct{}{}
		}
	}

	rowDelta := 0
	for parentID := range parents {
		delta, ok := m.replaceCachedMessageRows(parentID)
		if !ok {
			m.rebuildMessageCache()
			if oldScroll > 0 {
				m.scroll += len(m.renderedLines) - oldLineCount
			}
			m.clampScroll()
			return
		}
		rowDelta += delta
	}

	for _, packet := range packets {
		rowDelta += m.appendCachedMessageRows(packet)
	}
	if oldScroll > 0 {
		m.scroll += rowDelta
	}
	m.clampScroll()
}

func (m *model) appendCachedMessageRows(packet Packet) int {
	lineCount := max(1, m.renderPacketLineCount(packet, m.width))
	start := len(m.renderedLines)
	m.renderedLines = append(m.renderedLines, make([]string, lineCount)...)
	rows := messageRowRange{Start: start, End: start + lineCount}
	m.messageRowRanges = append(m.messageRowRanges, rows)
	if packet.ID != "" {
		if m.messageRowsByID == nil {
			m.messageRowsByID = make(map[string]messageRowRange)
		}
		m.messageRowsByID[packet.ID] = rows
	}
	m.renderWindowValid = false
	return lineCount
}

func replaceCachedRows[T any](source []T, start, end int, replacement []T) []T {
	updated := make([]T, 0, len(source)-(end-start)+len(replacement))
	updated = append(updated, source[:start]...)
	updated = append(updated, replacement...)
	updated = append(updated, source[end:]...)
	return updated
}

func (m *model) resizeCachedMessageRows(index, newCount int) int {
	if index < 0 || index >= len(m.messageRowRanges) {
		return 0
	}
	if newCount < 1 {
		newCount = 1
	}
	oldRows := m.messageRowRanges[index]
	messageID := m.messages[index].ID
	// A message can change its reply markers without changing its height.
	// Its cached body/link hitboxes must still be discarded in that case.
	delete(m.messageBodyRangesByID, messageID)
	delete(m.messageLinkRangesByID, messageID)

	delta := newCount - (oldRows.End - oldRows.Start)
	if delta == 0 {
		clear(m.renderedLines[oldRows.Start:oldRows.End])
		m.renderWindowValid = false
		return 0
	}
	m.renderedLines = replaceCachedRows(m.renderedLines, oldRows.Start, oldRows.End, make([]string, newCount))
	m.messageRowRanges[index] = messageRowRange{Start: oldRows.Start, End: oldRows.Start + newCount}
	for i := index + 1; i < len(m.messageRowRanges); i++ {
		m.messageRowRanges[i].Start += delta
		m.messageRowRanges[i].End += delta
	}
	for id, rows := range m.messageRowsByID {
		if id != messageID && rows.Start >= oldRows.End {
			rows.Start += delta
			rows.End += delta
			m.messageRowsByID[id] = rows
		}
	}
	m.messageRowsByID[messageID] = m.messageRowRanges[index]
	m.renderWindowValid = false
	return delta
}

func (m *model) replaceCachedMessageRows(messageID string) (int, bool) {
	if !m.renderCacheValid || m.renderCacheWidth != m.width {
		return 0, false
	}
	rows, exists := m.messageRowsByID[messageID]
	if !exists || rows.Start < 0 || rows.End > len(m.renderedLines) || rows.End <= rows.Start {
		return 0, false
	}
	index := sort.Search(len(m.messageRowRanges), func(i int) bool {
		return m.messageRowRanges[i].Start >= rows.Start
	})
	if index >= len(m.messageRowRanges) || m.messages[index].ID != messageID {
		return 0, false
	}
	message, exists := m.messageByID[messageID]
	if !exists {
		return 0, false
	}
	lineCount := m.renderPacketLineCount(message, m.width)
	delta := m.resizeCachedMessageRows(index, lineCount)
	if delta == 0 {
		clear(m.renderedLines[rows.Start:rows.End])
		m.renderWindowValid = false
	}
	return delta, true
}

func (m *model) rebuildMessageCache() {
	m.rebuildMessageIndexes()
	if m.width < 1 {
		m.renderedLines = nil
		m.messageRowsByID = nil
		m.messageRowRanges = nil
		m.messageBodyRangesByID = nil
		m.messageLinkRangesByID = nil
		m.renderWindowValid = false
		m.renderCacheValid = false
		return
	}

	m.messageBodyRangesByID = make(map[string][]messageBodyRange)
	m.messageLinkRangesByID = make(map[string][][]messageLinkRange)
	lines := make([]string, 0, len(m.messages))
	messageRows := make(map[string]messageRowRange, len(m.messages))
	rowRanges := make([]messageRowRange, len(m.messages))
	for index, message := range m.messages {
		start := len(lines)
		lineCount := max(1, m.renderPacketLineCount(message, m.width))
		lines = append(lines, make([]string, lineCount)...)
		rows := messageRowRange{Start: start, End: len(lines)}
		rowRanges[index] = rows
		if message.ID != "" {
			messageRows[message.ID] = rows
		}
	}
	m.renderedLines = lines
	m.messageRowsByID = messageRows
	m.messageRowRanges = rowRanges
	m.renderCacheWidth = m.width
	m.renderCacheValid = true
	m.renderWindowValid = false
	m.refreshRenderedWindow()
}

// refreshRenderedWindow styles only the visible rows plus a small three-row
// buffer. The row index still spans the full history for accurate scrolling.
func (m *model) refreshRenderedWindow() {
	if !m.renderCacheValid || m.renderCacheWidth != m.width || m.width < 1 {
		return
	}
	total := len(m.renderedLines)
	if total == 0 || len(m.messageRowRanges) == 0 {
		if m.renderWindowValid && m.renderWindowEnd > m.renderWindowStart {
			clear(m.renderedLines[m.renderWindowStart:m.renderWindowEnd])
		}
		m.renderWindowStart, m.renderWindowEnd = 0, 0
		m.renderWindowValid = true
		return
	}

	visibleHeight := max(1, m.messageHeight()-1)
	maxScroll := max(0, total-visibleHeight)
	scroll := min(max(m.scroll, 0), maxScroll)
	visibleStart := max(0, total-visibleHeight-scroll)
	visibleEnd := min(total, visibleStart+visibleHeight)
	requestedStart := max(0, visibleStart-3)
	requestedEnd := min(total, visibleEnd+3)
	first := sort.Search(len(m.messageRowRanges), func(i int) bool {
		return m.messageRowRanges[i].End > requestedStart
	})
	last := sort.Search(len(m.messageRowRanges), func(i int) bool {
		return m.messageRowRanges[i].Start >= requestedEnd
	})
	if first >= last {
		return
	}
	actualStart := m.messageRowRanges[first].Start
	actualEnd := m.messageRowRanges[last-1].End
	oldWindowValid := m.renderWindowValid && m.renderWindowEnd > m.renderWindowStart
	oldStart, oldEnd := m.renderWindowStart, m.renderWindowEnd
	if oldWindowValid && actualStart >= oldStart && actualEnd <= oldEnd {
		return
	}

	// Keep already-styled messages that remain in both windows. Small scroll
	// steps usually move the window by only one or two messages; clearing and
	// redrawing the full visible buffer would needlessly rerender all of them.
	overlapStart, overlapEnd := 0, 0
	if oldWindowValid {
		overlapStart = max(actualStart, oldStart)
		overlapEnd = min(actualEnd, oldEnd)
		if overlapStart >= overlapEnd {
			clear(m.renderedLines[oldStart:oldEnd])
			overlapStart, overlapEnd = 0, 0
		} else {
			if oldStart < overlapStart {
				clear(m.renderedLines[oldStart:overlapStart])
			}
			if overlapEnd < oldEnd {
				clear(m.renderedLines[overlapEnd:oldEnd])
			}
		}
	}

	for index := first; index < last; index++ {
		rows := m.messageRowRanges[index]
		if oldWindowValid && rows.Start >= overlapStart && rows.End <= overlapEnd {
			continue
		}
		rendered := m.renderPacketLines(m.messages[index], m.width)
		expected := rows.End - rows.Start
		if len(rendered) != expected {
			// Correct this message's index if the lightweight counter ever drifts
			// from the renderer, then retry with the updated global row offsets.
			m.resizeCachedMessageRows(index, len(rendered))
			m.refreshRenderedWindow()
			return
		}
		copy(m.renderedLines[rows.Start:rows.End], rendered)
	}
	m.renderWindowStart = actualStart
	m.renderWindowEnd = actualEnd
	m.renderWindowValid = true
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
	maxScroll := m.maxScroll()
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	m.refreshRenderedWindow()
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
		scrollFromTop := maxScroll - min(max(m.scroll, 0), maxScroll)
		top = scaleScrollbarPosition(scrollFromTop, maxScroll, maxTop)
	}
	return
}

func (m *model) setScrollFromTrack(trackPosition int) {
	total := len(m.renderMessageLines())
	_, thumbHeight, _ := m.scrollbarThumbForLineCount(total)
	m.setScrollFromThumbTopForLineCount(trackPosition-thumbHeight/2, total)
}

func (m *model) setScrollFromThumbTop(top int) {
	total := len(m.renderMessageLines())
	m.setScrollFromThumbTopForLineCount(top, total)
}

func (m *model) setScrollFromThumbTopForLineCount(top, total int) {
	_, thumbHeight, trackHeight := m.scrollbarThumbForLineCount(total)
	maxTop := trackHeight - thumbHeight
	if maxTop < 0 {
		maxTop = 0
	}
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}
	visible := max(1, m.messageHeight()-1)
	maxScroll := max(0, total-visible)
	if maxTop > 0 {
		m.scroll = maxScroll - scaleScrollbarPosition(top, maxTop, maxScroll)
	} else {
		m.scroll = 0
	}
	m.refreshRenderedWindow()
	m.refreshMessageHover()
}

func (m model) scrollbarHoldCmd(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return scrollbarHoldMsg{generation: generation}
	})
}
