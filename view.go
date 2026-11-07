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

// renderStatusBar keeps keyboard shortcuts and Quit together on the left,
// while anchoring the composer Send action to the right edge. Measured
// positions supply mouse hitboxes aligned with the rendered labels.
func (m model) renderStatusBar(usersOpen, splashOpen bool) (string, footerTargets) {
	availableWidth := max(1, m.width-1) // reserve one final cell as a styled blank
	lead := " " + m.status + " • "
	scrollLabel := "PgUp/PgDn"
	quitText := "Quit"
	if m.quitConfirm {
		quitText = "Sure?"
	}
	scrollTail := ": Scroll • " + quitText + " • Ctrl+C: "
	if usersOpen || splashOpen {
		scrollTail = ": Scroll • Esc: close • " + quitText + " • Ctrl+C: "
	}
	if m.themesOpen {
		switch {
		case m.themeEditField != "":
			scrollTail = ": Enter: confirm • Esc: cancel • " + quitText + " • Ctrl+C: "
		case m.themeDirty:
			scrollTail = ": Ctrl+S: save • Ctrl+T: Apply after save • Esc: back • " + quitText + " • Ctrl+C: "
		default:
			scrollTail = ": Ctrl+T: Apply • Esc: back • " + quitText + " • Ctrl+C: "
		}
	}
	prefix := lead + scrollLabel + scrollTail

	sendText := "[Send]"
	sendPrefix := "Enter: "
	includeSend := !usersOpen && (!splashOpen || m.themesOpen)
	includeApply := m.themesOpen && m.themeError == "" && m.themeEditField == ""

	sendTextWidth := lipgloss.Width(sendText)

	actionWidth := func() int {
		width := 0
		if includeSend {
			width += lipgloss.Width(sendPrefix) + sendTextWidth
		}
		return width
	}

	// Keep the composer action on the right; Quit stays beside Ctrl+C in
	// the shortcut text so the two keyboard hints remain together.
	if includeSend && lipgloss.Width(prefix)+actionWidth() > availableWidth {
		sendPrefix = ""
	}
	if actionWidth() > availableWidth && includeSend {
		includeSend = false
		sendPrefix = ""
	}

	rightActionsWidth := actionWidth()
	maxPrefixWidth := max(0, availableWidth-rightActionsWidth)
	pageLabelWidth := lipgloss.Width(scrollLabel)

	if !usersOpen && (!splashOpen || m.themesOpen) {
		// Keep the page controls visible where possible. Compress the
		// explanatory tail first, then the connection status.
		if lipgloss.Width(prefix) > maxPrefixWidth {
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
				prefix = ""
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
	pageStartByte := strings.Index(prefixVisible, scrollLabel)
	if pageStartByte >= 0 {
		pgUpEndByte := pageStartByte + len("PgUp")
		pgDnStartByte := pgUpEndByte + 1
		pgDnEndByte := pgDnStartByte + len("PgDn")
		highlights = append(highlights,
			footerHighlight{start: pageStartByte, end: pgUpEndByte, rendered: m.styles.accent.Render("PgUp")},
			footerHighlight{start: pgDnStartByte, end: pgDnEndByte, rendered: m.styles.accent.Render("PgDn")},
		)
	}
	applyStartByte := -1
	if includeApply {
		applyStartByte = strings.Index(prefixVisible, "Apply")
		if applyStartByte >= 0 {
			highlights = append(highlights, footerHighlight{
				start: applyStartByte, end: applyStartByte + len("Apply"), rendered: m.styles.accent.Render("Apply"),
			})
		}
	}
	quitStartByte := -1
	// The shortcut tail can be truncated before the full Ctrl+C label fits.
	// Find the visible Quit/Sure? label itself so its click target remains
	// valid even when only part of the following hint is shown.
	quitStartByte = strings.LastIndex(prefixVisible, quitText)
	if quitStartByte >= 0 {
		highlights = append(highlights, footerHighlight{
			start: quitStartByte, end: quitStartByte + len(quitText), rendered: m.styles.accent.Render(quitText),
		})
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

	gapWidth := max(0, availableWidth-prefixWidth-rightActionsWidth)
	var targets footerTargets
	if pageStartByte >= 0 {
		pageStart := lipgloss.Width(prefixVisible[:pageStartByte])
		targets.hasPageScroll = true
		targets.pgUpStart = pageStart
		targets.pgUpEnd = pageStart + lipgloss.Width("PgUp")
		targets.pgDnStart = pageStart + lipgloss.Width("PgUp/")
		targets.pgDnEnd = targets.pgDnStart + lipgloss.Width("PgDn")
	}
	if applyStartByte >= 0 {
		targets.hasApply = true
		targets.applyStart = lipgloss.Width(prefixVisible[:applyStartByte])
		targets.applyEnd = targets.applyStart + lipgloss.Width("Apply")
	}
	if quitStartByte >= 0 {
		targets.quitStart = lipgloss.Width(prefixVisible[:quitStartByte])
		targets.quitEnd = targets.quitStart + lipgloss.Width(quitText)
	}

	var line strings.Builder
	line.WriteString(renderedPrefix)
	if gapWidth > 0 {
		line.WriteString(m.styles.status.Render(strings.Repeat("⠀", gapWidth)))
	}
	position := prefixWidth + gapWidth

	if includeSend {
		sendPrefixWidth := lipgloss.Width(sendPrefix)
		targets.hasSend = true
		targets.sendStart = position + sendPrefixWidth
		targets.sendEnd = targets.sendStart + sendTextWidth
		if sendPrefix != "" {
			line.WriteString(m.styles.status.Render(sendPrefix))
		}
		if m.sendFeedback {
			line.WriteString(m.styles.status.Render(sendText))
		} else {
			line.WriteString(m.styles.accent.Render(sendText))
		}
	}

	// Keep the final inset cell painted while the composer action remains
	// anchored to the right edge across terminal resizes.
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
