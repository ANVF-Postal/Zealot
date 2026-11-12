package main

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// -----------------------------------------------------------------------------
// Typing indicator
// -----------------------------------------------------------------------------

func (m model) typingText() string {
	if len(m.typingOrder) == 0 {
		return ""
	}

	names := make([]string, 0, len(m.typingOrder))
	for _, name := range m.typingOrder {
		if _, ok := m.typing[name]; ok {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0] + " is typing…"
	}
	if len(names) == 2 {
		return names[0] + " and " + names[1] + " are typing…"
	}
	return fmt.Sprintf("%d people are typing…", len(names))
}

// -----------------------------------------------------------------------------
// Header
// -----------------------------------------------------------------------------

func (m model) renderHeader() string {
	if m.width < 1 {
		return ""
	}

	title := "ZEALOT"
	if m.roomExplicit {
		title = "ZEALOT — " + m.room
	}
	rightLabel := fmt.Sprintf("Online: %d", m.onlineCount)
	if m.usersOpen || m.splashOpen {
		rightLabel = "[X]"
	}

	// Reserve the right edge for the online button, then fill every remaining
	// cell with the same configured background. No nested styles are used.
	rightLabel = truncateANSI(rightLabel, m.width)
	titleWidth := max(0, m.width-lipgloss.Width(rightLabel))
	title = truncateANSI(title, titleWidth)
	gapWidth := max(0, m.width-lipgloss.Width(title)-lipgloss.Width(rightLabel))
	line := title + strings.Repeat(" ", gapWidth) + rightLabel
	if remaining := m.width - lipgloss.Width(line); remaining > 0 {
		line += strings.Repeat(" ", remaining)
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
		Background(lipgloss.Color(m.cfg.Theme.Background)).
		Render(line)
}

// -----------------------------------------------------------------------------
// Main UI
// -----------------------------------------------------------------------------

func appView(content string, hoverEnabled bool) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	// Passive pointer motion is needed only for message-body hover in the
	// main chat. Panels have no hover targets, so keep click, wheel, and drag
	// support without subscribing to every mouse movement.
	v.MouseMode = tea.MouseModeCellMotion
	if hoverEnabled {
		v.MouseMode = tea.MouseModeAllMotion
	}
	return v
}

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return appView("Starting Zealot…", false)
	}

	// -------------------------------------------------------------------------
	// Header
	// -------------------------------------------------------------------------

	header := m.renderHeader()

	// -------------------------------------------------------------------------
	// Alternate panels
	// -------------------------------------------------------------------------

	if m.longReader != nil {
		return m.renderViewWithChat(header, m.renderLongReaderPanel(), false, false)
	}

	if m.usersOpen {
		// The existing chat pane becomes the roster. The heading uses a clean
		// full-width background row; the scrollbar occupies the far-right
		// interior cell on user rows only.
		totalUserRows := m.userRowCount()
		visibleUsers := m.usersPanelHeight()
		userStart := min(m.usersScroll, totalUserRows)
		userEnd := min(userStart+visibleUsers, totalUserRows)
		visible := m.userRowsRange(userStart, userEnd)

		bg := lipgloss.Color(m.cfg.Theme.Background)
		rowFill := lipgloss.NewStyle().Background(bg)
		scrollbarStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.BorderColor)).
			Background(bg)
		contentWidth := chatInnerWidth(m.width)
		textWidth := max(1, contentWidth-1)

		rosterRows := make([]string, 0, m.messageHeight())
		heading := m.styles.accent.Render("  Online users:")
		heading = padANSIToWidth(heading, textWidth, rowFill)
		rosterRows = append(rosterRows, heading+scrollbarStyle.Render("^"))

		for len(visible) < visibleUsers {
			visible = append(visible, "")
		}
		railTop, thumbHeight, trackHeight := m.usersScrollbarThumbForRowCount(totalUserRows)
		for i, line := range visible {
			text := padANSIToWidth(line, textWidth, rowFill)
			glyph := "│"
			switch {
			case i == visibleUsers-1:
				glyph = "v"
			case totalUserRows > visibleUsers && i < trackHeight &&
				i >= railTop && i < railTop+thumbHeight:
				glyph = "█"
			}
			rosterRows = append(rosterRows, text+scrollbarStyle.Render(glyph))
		}
		for len(rosterRows) < m.messageHeight() {
			rosterRows = append(rosterRows, padANSIToWidth("", contentWidth, rowFill))
		}
		if len(rosterRows) > m.messageHeight() {
			rosterRows = rosterRows[:m.messageHeight()]
		}
		for i := range rosterRows {
			rosterRows[i] = renderChatRow(rowFill.Width(contentWidth), rosterRows[i], contentWidth)
		}
		chat := strings.Join(rosterRows, "\n")
		chatBox := m.chatBorderStyle().
			Width(m.width).
			Height(m.messageHeight()).
			Render(chat)
		return m.renderViewWithChat(header, chatBox, true, false)
	}

	if m.commandsOpen {
		chatBox := m.renderCommandsBrowser()
		return m.renderViewWithChat(header, chatBox, false, false)
	}

	if m.themesOpen {
		chatBox := m.renderThemeBrowser()
		return m.renderViewWithChat(header, chatBox, false, true)
	}

	if m.splashOpen {
		chatBox := m.renderSplashPanel()
		return m.renderViewWithChat(header, chatBox, false, true)
	}

	// The chat history is not needed by either alternate panel. Avoid
	// formatting every stored message while the splash/users view is active.
	lines := m.renderMessageLines()
	chatHeight := m.messageHeight() - 1
	if chatHeight < 1 {
		chatHeight = 1
	}

	start := len(lines) -
		chatHeight -
		m.scroll

	if start < 0 {
		start = 0
	}

	end := start + chatHeight

	if end > len(lines) {
		end = len(lines)
	}

	// Copy the visible window before padding it; lines may be the cached slice,
	// and appending to a view into that slice could overwrite cached history.
	visible := append([]string(nil), lines[start:end]...)
	chatBlank := lipgloss.NewStyle().
		Width(chatInnerWidth(m.width)).
		Background(lipgloss.Color(m.cfg.Theme.Background)).
		Render("")

	for len(visible) < chatHeight {
		visible = append(visible, chatBlank)
	}

	// Reserve the final interior column for a permanent scrollbar rail.
	// The thumb only appears once the message history exceeds the viewport.
	thumbTop, thumbHeight, trackHeight := m.scrollbarThumbForLineCount(len(lines))
	visibleHeight := max(1, m.messageHeight()-1)
	maxScroll := max(0, len(lines)-visibleHeight)
	scrollbarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.BorderColor)).
		Background(lipgloss.Color(m.cfg.Theme.Background))
	scrollbarRows := make([]string, chatHeight+1)
	for i := range scrollbarRows {
		switch {
		case i == 0:
			scrollbarRows[i] = scrollbarStyle.Render("^")
		case i == chatHeight:
			scrollbarRows[i] = scrollbarStyle.Render("v")
		default:
			trackPosition := i-1
			glyph := "│"
			if maxScroll > 0 && trackPosition >= thumbTop && trackPosition < thumbTop+thumbHeight {
				glyph = "█"
			}
			if trackPosition >= trackHeight {
				glyph = " "
			}
			scrollbarRows[i] = scrollbarStyle.Render(glyph)
		}
	}
	for i := range visible {
		visible[i] = truncateANSI(visible[i], chatContentWidth(m.width)) + scrollbarRows[i]
	}
	chat := strings.Join(visible, "\n")

	// -------------------------------------------------------------------------
	// Permanent typing line
	// -------------------------------------------------------------------------

	typing := m.typingText()
	typingLine := chatBlank
	if typing != "" {
		typingLine = m.styles.accent.Width(chatContentWidth(m.width)).Render("  " + typing)
	}
	// A private notification shares the permanent typing row, but only the
	// addressed client receives the event. Give it priority if both exist.
	for _, name := range m.privateTypingOrder {
		if user, ok := m.privateTyping[name]; ok {
			typingLine = lipgloss.NewStyle().
				Foreground(lipgloss.Color(user.Color)).
				Background(lipgloss.Color(m.cfg.Theme.Background)).
				Width(chatContentWidth(m.width)).
				Render("  " + terminalSafeText(user.Name) + " is whispering to you…")
			break
		}
	}
	typingLine = truncateANSI(typingLine, chatContentWidth(m.width)) + scrollbarRows[chatHeight]
	chat += "\n" + typingLine

	chatBox := m.chatBorderStyle().
		Width(m.width).
		Height(m.messageHeight()).
		Render(chat)

	return m.renderViewWithChat(header, chatBox, false, false)
}


 
// renderSplashPanel replaces only the chat pane with the splash page. Its
// logo uses the header accent; all information below it uses normal chat text.
func (m model) renderSplashPanel() string {
	lines := m.splashRows()
	visibleHeight := m.splashPanelHeight()
	start := m.splashScroll
	if start < 0 { start = 0 }
	if maxStart := max(0, len(lines)-visibleHeight); start > maxStart { start = maxStart }
	end := min(start+visibleHeight, len(lines))
	visible := append([]string(nil), lines[start:end]...)

	bg := lipgloss.Color(m.cfg.Theme.Background)
	rowFill := lipgloss.NewStyle().Background(bg)
	scrollbarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.BorderColor)).
		Background(bg)
	contentWidth := chatInnerWidth(m.width)
	textWidth := max(1, contentWidth-1)

	for len(visible) < visibleHeight {
		visible = append(visible, "")
	}
	maxScroll := max(0, len(lines)-visibleHeight)
	top, thumbHeight, trackHeight := m.splashScrollbarThumbForRowCount(len(lines))
	rows := make([]string, 0, visibleHeight)
	for i, line := range visible {
		glyph := "│"
		switch {
		case i == 0:
			glyph = "^"
		case i == visibleHeight-1:
			glyph = "v"
		default:
			trackPosition := i - 1
			if maxScroll > 0 && trackPosition >= top && trackPosition < top+thumbHeight {
				glyph = "█"
			}
			if trackPosition >= trackHeight {
				glyph = " "
			}
		}
		text := padANSIToWidth(line, textWidth, rowFill)
		rows = append(rows, renderChatRow(rowFill.Width(contentWidth), text+scrollbarStyle.Render(glyph), contentWidth))
	}
	return m.chatBorderStyle().
		Width(m.width).
		Height(m.messageHeight()).
		Render(strings.Join(rows, "\n"))
}

type footerTargets struct {
	quitStart  int
	quitEnd    int
	sendStart  int
	sendEnd    int
	applyStart int
	applyEnd   int
	pgUpStart  int
	pgUpEnd    int
	pgDnStart  int
	pgDnEnd    int
	hasSend    bool
	hasApply   bool
	hasPageScroll bool
}

// compactFooterStatusLead shortens the Connected label from the right while
// preserving its bullet separator from the page-scroll controls.
func compactFooterStatusLead(status string, maxWidth int) string {
	const separator = " • "
	fixedWidth := 1 + len([]rune(separator)) // leading space + separator
	statusWidth := maxWidth - fixedWidth
	if statusWidth < 1 {
		return ""
	}
	shortStatus := truncateANSI(status, statusWidth)
	if shortStatus == "" {
		return ""
	}
	// truncateANSI appends a reset when it clips text. Remove that reset
	// before adding the separator so the surrounding status style continues
	// to paint the separator spaces with the configured background.
	shortStatus = strings.TrimSuffix(shortStatus, "\x1b[0m")
	return " " + shortStatus + separator
}

// renderThemeShortcutFooter follows the ordinary footer's compression order:
// remove Enter:, clip non-button shortcuts from the right, shorten the status
// while preserving complete PgUp/PgDn buttons, and only then drop page scroll.
// All mouse targets are derived from the same rendered cursor positions.
func (m model) renderThemeShortcutFooter() (string, footerTargets) {
	available := max(1, m.width-1) // one final styled inset cell
	const page = "PgUp/PgDn"
	const hint = ": Ctrl+T:"
	const tail = " • Esc: back • Ctrl+C:"
	const apply = " Apply"
	const send = "[Send]"

	targets := footerTargets{}
	quitText := "Quit"
	if m.quitConfirm {
		quitText = "Sure?"
	}
	lead := " " + m.status + " • "
	fullPrefix := lead + page + hint
	quitLeftPad, quitRightPad := 1, 1
	applyVisible := true
	sendVisible := true

	// Keep a separator between the clickable actions. At impossible
	// widths drop Send, then Apply, before clipping Quit itself.
	const actionSeparator = 1
	minAll := lipgloss.Width(apply) + lipgloss.Width(quitText) +
		lipgloss.Width(send) + actionSeparator
	if available < minAll {
		sendVisible = false
	}
	if available < lipgloss.Width(apply)+lipgloss.Width(quitText)+actionSeparator {
		applyVisible = false
	}
	if available < lipgloss.Width(quitText) {
		quitText = truncateANSI(quitText, available)
	}
	minActions := lipgloss.Width(quitText)
	if applyVisible {
		minActions += lipgloss.Width(apply) + actionSeparator
	}
	if sendVisible {
		minActions += lipgloss.Width(send)
	}
	// Retain the left pad when Apply is visible; it separates Apply
	// from Quit. The right pad gives Quit breathing room beside Send.
	if available < minActions+quitRightPad {
		quitRightPad = 0
	}
	if !applyVisible && available < minActions+quitLeftPad+quitRightPad {
		quitLeftPad = 0
	}
	applyText := ""
	if applyVisible {
		applyText = apply
	}
	sendWidth := 0
	if sendVisible {
		sendWidth = lipgloss.Width(send)
	}

	// Once Enter: is removed, Send remains anchored at the right edge.
	// The space between Quit and Send is not a shortcut and is protected.
	fullWidth := lipgloss.Width(fullPrefix+apply+tail) +
		quitLeftPad + lipgloss.Width(quitText) + quitRightPad +
		lipgloss.Width("Enter: "+send)
	sendPrefix := ""
	if sendVisible && available >= fullWidth {
		sendPrefix = "Enter: "
	}
	budget := max(0, available-lipgloss.Width(applyText)-
		quitLeftPad-lipgloss.Width(quitText)-quitRightPad-
		sendWidth-lipgloss.Width(sendPrefix))

	// The right-side hint is clipped first, from its end. Next the
	// Ctrl+T label shrinks, followed by the Connected status. Never clip
	// the clickable page labels themselves: either show both or neither.
	prefix := ""
	tailText := ""
	if budget >= lipgloss.Width(fullPrefix) {
		prefix = fullPrefix
		tailText = truncateANSI(tail, budget-lipgloss.Width(prefix))
	} else if budget >= lipgloss.Width(lead+page) {
		prefix = lead + page + truncateANSI(hint, budget-lipgloss.Width(lead+page))
	} else if budget >= lipgloss.Width(page) {
		prefix = compactFooterStatusLead(m.status, budget-lipgloss.Width(page)) + page
	}
	prefix = strings.TrimSuffix(prefix, "\x1b[0m")
	tailText = strings.TrimSuffix(tailText, "\x1b[0m")

	var line strings.Builder
	cursor := 0
	pageIndex := strings.Index(prefix, page)
	if pageIndex >= 0 {
		beforePage := prefix[:pageIndex]
		line.WriteString(m.styles.status.Render(beforePage))
		cursor += lipgloss.Width(beforePage)
		targets.hasPageScroll = true
		targets.pgUpStart = cursor
		targets.pgUpEnd = cursor + lipgloss.Width("PgUp")
		targets.pgDnStart = targets.pgUpEnd + 1
		targets.pgDnEnd = targets.pgDnStart + lipgloss.Width("PgDn")
		line.WriteString(m.styles.accent.Render("PgUp"))
		line.WriteString(m.styles.status.Render("/"))
		line.WriteString(m.styles.accent.Render("PgDn"))
		cursor += lipgloss.Width(page)
		line.WriteString(m.styles.status.Render(prefix[pageIndex+len(page):]))
		cursor = lipgloss.Width(prefix)
	} else {
		line.WriteString(m.styles.status.Render(prefix))
		cursor = lipgloss.Width(prefix)
	}

	if applyVisible {
		line.WriteString(m.styles.status.Render(" "))
		cursor++
		targets.hasApply = true
		targets.applyStart = cursor
		targets.applyEnd = cursor + lipgloss.Width("Apply")
		line.WriteString(m.styles.accent.Render("Apply"))
		cursor = targets.applyEnd
	}
	line.WriteString(m.styles.status.Render(tailText))
	cursor += lipgloss.Width(tailText)
	if quitLeftPad > 0 {
		line.WriteString(m.styles.status.Render(strings.Repeat(" ", quitLeftPad)))
		cursor += quitLeftPad
	}
	targets.quitStart = cursor
	targets.quitEnd = cursor + lipgloss.Width(quitText)
	line.WriteString(m.styles.accent.Render(quitText))
	cursor = targets.quitEnd
	if quitRightPad > 0 {
		line.WriteString(m.styles.status.Render(strings.Repeat(" ", quitRightPad)))
		cursor += quitRightPad
	}

	gap := max(0, available-cursor-lipgloss.Width(sendPrefix)-sendWidth)
	line.WriteString(m.styles.status.Render(strings.Repeat("⠀", gap)))
	if sendVisible {
		line.WriteString(m.styles.status.Render(sendPrefix))
		targets.hasSend = true
		targets.sendStart = available-lipgloss.Width(send)
		targets.sendEnd = available
		if m.sendFeedback {
			line.WriteString(m.styles.status.Render(send))
		} else {
			line.WriteString(m.styles.accent.Render(send))
		}
	}
	line.WriteString(m.styles.status.Render("⠀"))
	return line.String(), targets
}

// renderStatusBar keeps ordinary shortcuts on the left, the Quit button
// immediately after the Ctrl+C hint, and the composer Send action anchored
// to the right edge. The measured layout supplies matching mouse hitboxes.
func (m model) renderStatusBar(usersOpen, splashOpen bool) (string, footerTargets) {
	if m.themesOpen && m.themeIncoming == nil && !m.themeShareConfirm && m.themeError == "" && m.themeEditField == "" && !m.themeDirty && !m.themePacking && m.width >= 12 {
		return m.renderThemeShortcutFooter()
	}
	availableWidth := max(1, m.width-1) // reserve one final cell as a styled blank
	lead := " " + m.status + " • "
	scrollLabel := "PgUp/PgDn"
	scrollTail := ": Scroll • Ctrl+C: "
	if usersOpen || splashOpen || m.longReader != nil || m.commandsOpen {
		// Keep the secondary views' keyboard shortcut wording consistent.
		scrollTail = ": Scroll • Esc: close • Ctrl+C: "
	}
	includeApply := m.themesOpen && m.themeIncoming == nil && !m.themeShareConfirm && m.themeError == "" && m.themeEditField == "" && !m.themeDirty && !m.themePacking
	if m.themesOpen {
		switch {
		case m.themeIncoming != nil:
			scrollTail = ": Esc: close preview • Ctrl+C: "
		case m.themeShareConfirm:
			scrollTail = ": Enter: share • Esc: cancel • Ctrl+C: "
		case m.themePackNaming:
			scrollLabel = "Pack"
			scrollTail = ": Enter: save pack • Esc: cancel • Ctrl+C: "
		case m.themePacking:
			scrollTail = ": Space: select • Enter: confirm • Esc: cancel • Ctrl+C: "
		case m.themeEditField != "":
			scrollTail = ": Enter: confirm • Esc: cancel • Ctrl+C: "
		case m.themeDirty:
			scrollTail = ": Ctrl+S: save • Esc: back • Ctrl+C: "
		default:
			scrollTail = ": Ctrl+T: • Esc: back • Ctrl+C: "
		}
	}
	prefix := lead + scrollLabel + scrollTail

	quitText := "Quit"
	if m.quitConfirm {
		quitText = "Sure?"
	}

	targets := footerTargets{}
	includeSend := !m.themePacking && m.themeIncoming == nil && !m.themeShareConfirm && !usersOpen && (!splashOpen || m.themesOpen)
	sendPrefix := "Enter: "
	sendText := "[Send]"
	quitWidth := lipgloss.Width(quitText)
	applyWidth := 0
	if includeApply {
		applyWidth = lipgloss.Width(" Apply")
		// Only yield Apply when the terminal cannot physically fit both
		// action labels; otherwise keep it reserved through all resizing.
		if availableWidth < quitWidth+applyWidth {
			includeApply = false
			applyWidth = 0
		}
	}
	fullSendWidth := lipgloss.Width(sendPrefix + sendText)
	shortSendWidth := lipgloss.Width(sendText)
	quitLeftPad := 1
	quitRightPad := 1

	// Preserve the footer resize priority: remove "Enter: " first, keep the
	// fixed action buttons together, compact Quit padding, then compress the
	// shortcut prefix. At extreme widths, Send then Apply yield to Quit.
	if includeSend && lipgloss.Width(prefix)+quitWidth+applyWidth+2+fullSendWidth > availableWidth {
		sendPrefix = ""
	}

	sendWidth := 0
	if includeSend {
		sendWidth = lipgloss.Width(sendPrefix) + shortSendWidth
		for availableWidth < quitWidth+applyWidth+sendWidth+quitLeftPad+quitRightPad {
			if quitLeftPad > 0 {
				quitLeftPad = 0
				continue
			}
			if quitRightPad > 0 {
				quitRightPad = 0
				continue
			}
			break
		}
		if availableWidth < quitWidth+applyWidth+sendWidth {
			includeSend = false
			sendPrefix = ""
			sendWidth = 0
		}
	}
	if availableWidth < quitWidth+applyWidth+quitLeftPad+quitRightPad {
		if quitLeftPad > 0 {
			quitLeftPad = 0
		}
		if availableWidth < quitWidth+applyWidth+quitRightPad {
			quitRightPad = 0
		}
	}
	// Keep Apply in the fixed footer action layout even when space is tight.
	// The shortcut prefix and its status lead compress before this action yields.
	quitButtonWidth := quitWidth + quitLeftPad + quitRightPad
	maxPrefixWidth := availableWidth - quitButtonWidth - applyWidth - sendWidth
	if maxPrefixWidth < 0 {
		maxPrefixWidth = 0
	}

	pageLabelWidth := lipgloss.Width(scrollLabel)
	if !usersOpen && (!splashOpen || m.themesOpen) {
		if lipgloss.Width(prefix) > maxPrefixWidth {
			if m.themesOpen && includeApply {
				// Keep the theme Apply shortcut reachable as the footer narrows.
				// Compress the status and trailing hints before clipping Ctrl+T.
				const applyShortcut = ": Ctrl+T:"
				coreWidth := lipgloss.Width(scrollLabel + applyShortcut)
				if maxPrefixWidth >= coreWidth {
					prefix = scrollLabel + applyShortcut
					remainder := strings.TrimPrefix(scrollTail, applyShortcut)
					remainingWidth := maxPrefixWidth - coreWidth
					if remainingWidth > 0 {
						prefix += truncateANSI(remainder, remainingWidth)
					}
				} else {
					// At very narrow widths, move Apply into the fixed action
					// cluster beside Quit rather than dropping the button.
					prefix = ""
				}
			} else {
				tailBudget := maxPrefixWidth - lipgloss.Width(lead) - pageLabelWidth
				if tailBudget >= 0 {
					prefix = lead + scrollLabel
					if tailBudget > 0 {
						prefix += truncateANSI(scrollTail, tailBudget)
					}
				} else if maxPrefixWidth >= pageLabelWidth {
					statusBudget := maxPrefixWidth - pageLabelWidth
					prefix = compactFooterStatusLead(m.status, statusBudget) + scrollLabel
				} else {
					// Never show a partial page label as though it were clickable.
					prefix = ""
				}
			}
		}
	} else {
		pageLabelStart := lipgloss.Width(lead)
		if maxPrefixWidth < pageLabelStart+pageLabelWidth+1 {
			prefix = ""
		} else {
			prefix = truncateANSI(prefix, maxPrefixWidth)
		}
	}

	prefixVisible := strings.TrimRight(prefix, " ")
	prefixWidth := lipgloss.Width(prefixVisible)

	type footerHighlight struct {
		start    int
		end      int
		rendered string
	}
	var highlights []footerHighlight

	pageStartByte := -1
	if !m.themePackNaming {
		pageStartByte = strings.Index(prefixVisible, scrollLabel)
	}
	if pageStartByte >= 0 {
		pgUpEndByte := pageStartByte + len("PgUp")
		pgDnStartByte := pgUpEndByte + 1
		pgDnEndByte := pgDnStartByte + len("PgDn")
		highlights = append(highlights,
			footerHighlight{start: pageStartByte, end: pgUpEndByte, rendered: m.styles.accent.Render("PgUp")},
			footerHighlight{start: pgDnStartByte, end: pgDnEndByte, rendered: m.styles.accent.Render("PgDn")},
		)
		targets.hasPageScroll = true
		pageStart := lipgloss.Width(prefixVisible[:pageStartByte])
		targets.pgUpStart = pageStart
		targets.pgUpEnd = pageStart + lipgloss.Width("PgUp")
		targets.pgDnStart = pageStart + lipgloss.Width("PgUp/")
		targets.pgDnEnd = targets.pgDnStart + lipgloss.Width("PgDn")
	}

	applyInline := false
	if includeApply {
		if applyInsertByte := strings.Index(prefixVisible, "Ctrl+T:"); applyInsertByte >= 0 {
			applyInline = true
			applyInsertByte += len("Ctrl+T:")
			applyStart := lipgloss.Width(prefixVisible[:applyInsertByte]) + 1
			targets.hasApply = true
			targets.applyStart = applyStart
			targets.applyEnd = applyStart + lipgloss.Width("Apply")
			highlights = append(highlights, footerHighlight{
				start: applyInsertByte,
				end: applyInsertByte,
				rendered: m.styles.status.Render(" ") + m.styles.accent.Render("Apply"),
			})
		}
	}
	if includeApply && !applyInline {
		targets.hasApply = true
		targets.applyStart = prefixWidth + 1
		targets.applyEnd = targets.applyStart + lipgloss.Width("Apply")
	}

	sort.Slice(highlights, func(i, j int) bool { return highlights[i].start < highlights[j].start })
	var prefixBuilder strings.Builder
	cursor := 0
	for _, highlight := range highlights {
		if highlight.start < cursor {
			continue
		}
		prefixBuilder.WriteString(m.styles.status.Render(prefixVisible[cursor:highlight.start]))
		prefixBuilder.WriteString(highlight.rendered)
		cursor = highlight.end
	}
	prefixBuilder.WriteString(m.styles.status.Render(prefixVisible[cursor:]))
	renderedPrefix := prefixBuilder.String()

	gapWidth := availableWidth - prefixWidth - quitButtonWidth - applyWidth - sendWidth
	if gapWidth < 0 {
		gapWidth = 0
	}

	// Apply is rendered inline after Ctrl+T in the shortcut hint. Its width
	// remains reserved above so the Quit and Send buttons keep their layout.
	position := prefixWidth + applyWidth
	var line strings.Builder
	line.WriteString(renderedPrefix)
	if includeApply && !applyInline {
		line.WriteString(m.styles.status.Render(" "))
		line.WriteString(m.styles.accent.Render("Apply"))
	}
	if quitLeftPad > 0 {
		line.WriteString(m.styles.status.Render(strings.Repeat(" ", quitLeftPad)))
		position += quitLeftPad
	}
	targets.quitStart = position
	targets.quitEnd = targets.quitStart + quitWidth
	line.WriteString(m.styles.accent.Render(quitText))
	position += quitWidth
	if quitRightPad > 0 {
		line.WriteString(m.styles.status.Render(strings.Repeat(" ", quitRightPad)))
		position += quitRightPad
	}
	// Styled braille blanks force repainting when the footer changes length.
	line.WriteString(m.styles.status.Render(strings.Repeat("⠀", gapWidth)))

	if includeSend {
		targets.hasSend = true
		targets.sendStart = availableWidth - shortSendWidth
		targets.sendEnd = availableWidth
		if sendPrefix != "" {
			line.WriteString(m.styles.status.Render(sendPrefix))
		}
		if m.sendFeedback {
			line.WriteString(m.styles.status.Render(sendText))
		} else {
			line.WriteString(m.styles.accent.Render(sendText))
		}
	}

	// Paint the final inset cell while keeping [Send] one cell from the edge.
	line.WriteString(m.styles.status.Render("⠀"))
	return line.String(), targets
}

// renderViewWithChat keeps the application shell unchanged while swapping the
// contents of the chat pane for the online roster or splash page.
func (m model) renderViewWithChat(header, chatBox string, usersOpen, splashOpen bool) tea.View {
	// -------------------------------------------------------------------------
	// Input
	// -------------------------------------------------------------------------

	// Input content rows are configurable; borders add two more rows.
	inputView := strings.Split(strings.TrimSuffix(m.input.View(), "\n"), "\n")
	if len(inputView) == 0 {
		inputView = []string{""}
	}
	mentionStyle := m.mentionStyle(lipgloss.Color(m.cfg.Theme.Background), false)
	mentionCandidates := m.mentionCandidates()
	for i := range inputView {
		inputView[i] = styleANSIVisibleMentions(inputView[i], mentionCandidates, mentionStyle)
	}
	prompt := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
		Background(lipgloss.Color(m.cfg.Theme.Background)).
		Bold(true).
		Render("> ")
	firstInputRow := inputView[0]
	inputView[0] = prompt
	if m.replyTargetID != "" {
		inputView[0] += m.styles.accent.Render(replyComposerPrefix)
	}
	inputView[0] += firstInputRow

	// The textarea emits shorter/unstyled end-of-buffer rows when empty. Fill
	// every row's remaining columns with the configured background so those
	// rows match the rest of the composer before the user starts typing.
	inputInnerWidth := m.width - 4 // side borders (2) + composer padding (2)
	if narrowBordersHidden(m.width) {
		inputInnerWidth += 2
	}
	if inputInnerWidth < 1 {
		inputInnerWidth = 1
	}
	inputBackground := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Theme.Background))
	if strings.TrimSpace(m.input.Value()) == "" {
		// Bubbles' empty-placeholder view uses EndOfBuffer cells for rows below
		// the placeholder. Those cells can retain a terminal-default background
		// through the viewport, even when EndOfBuffer is styled. Replace the
		// lower empty rows wholesale with explicitly background-painted spaces.
		for i := 1; i < len(inputView); i++ {
			inputView[i] = inputBackground.Render(strings.Repeat(" ", inputInnerWidth))
		}
	}
	for i, line := range inputView {
		remaining := inputInnerWidth - lipgloss.Width(line)
		if remaining > 0 {
			inputView[i] = line + inputBackground.Render(strings.Repeat(" ", remaining))
		}
	}
	inputHeight := max(1, m.input.Height())
	if len(inputView) > inputHeight {
		inputView = inputView[:inputHeight]
	}
	for len(inputView) < inputHeight {
		inputView = append(inputView, inputBackground.Render(strings.Repeat(" ", inputInnerWidth)))
	}
	inputContent := strings.Join(inputView, "\n")
	inputBox := m.inputBoxStyle().
		Width(m.width).
		Height(inputHeight).
		Render(inputContent)

	// -------------------------------------------------------------------------
	// Status
	// -------------------------------------------------------------------------

	parts := []string{header, chatBox, inputBox}
	if m.statusBarVisible() {
		statusText, _ := m.renderStatusBar(usersOpen, splashOpen && !m.themesOpen)
		status := m.styles.status.Render(statusText)
		parts = append(parts, status)
	}

	// -------------------------------------------------------------------------
	// Full application
	// -------------------------------------------------------------------------

	// Each section is already rendered to its final terminal dimensions and
	// paints its own foreground/background. Running the completed screen
	// through another Width/Height Style.Render would make Lipgloss measure
	// and wrap the entire ANSI-rich view again on every redraw.
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return appView(content, !usersOpen && !splashOpen && len(m.messages) > 0)
}
