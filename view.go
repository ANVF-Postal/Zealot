package main

import (
	"fmt"
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
		Foreground(lipgloss.Color(m.cfg.AccentColor)).
		Background(lipgloss.Color(m.cfg.Background)).
		Render(line)
}

// -----------------------------------------------------------------------------
// Main UI
// -----------------------------------------------------------------------------

func appView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func (m model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return appView("Starting Zealot…")
	}

	// -------------------------------------------------------------------------
	// Header
	// -------------------------------------------------------------------------

	header := m.renderHeader()

	// -------------------------------------------------------------------------
	// Chat
	// -------------------------------------------------------------------------

	lines := m.renderMessageLines()

	// ALWAYS reserve one line for the typing indicator.
	//
	// When nobody is typing, this line is blank.
	// When somebody starts typing, the indicator appears on exactly that
	// same line.
	chatHeight := m.messageHeight() - 1

	if chatHeight < 1 {
		chatHeight = 1
	}

	if m.usersOpen {
		// The existing chat pane becomes the roster. The heading uses a clean
		// full-width background row; the scrollbar occupies the far-right
		// interior cell on user rows only.
		users := m.userRows()
		visibleUsers := m.usersPanelHeight()
		userStart := min(m.usersScroll, len(users))
		userEnd := min(userStart+visibleUsers, len(users))
		visible := append([]string(nil), users[userStart:userEnd]...)

		bg := lipgloss.Color(m.cfg.Background)
		rowFill := lipgloss.NewStyle().Background(bg)
		scrollbarStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.BorderColor)).
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
		railTop, thumbHeight, trackHeight := m.usersScrollbarThumb()
		for i, line := range visible {
			text := padANSIToWidth(line, textWidth, rowFill)
			glyph := "│"
			switch {
			case i == visibleUsers-1:
				glyph = "v"
			case len(users) > visibleUsers && i < trackHeight &&
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

	if m.splashOpen {
		chatBox := m.renderSplashPanel()
		return m.renderViewWithChat(header, chatBox, false, true)
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
		Background(lipgloss.Color(m.cfg.Background)).
		Render("")

	for len(visible) < chatHeight {
		visible = append(visible, chatBlank)
	}

	// Reserve the final interior column for a permanent scrollbar rail.
	// The thumb only appears once the message history exceeds the viewport.
	_, thumbHeight, trackHeight := m.scrollbarThumb()
	thumbTop, _, _ := m.scrollbarThumb()
	scrollbarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.BorderColor)).
		Background(lipgloss.Color(m.cfg.Background))
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
			if m.maxScroll() > 0 && trackPosition >= thumbTop && trackPosition < thumbTop+thumbHeight {
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

	bg := lipgloss.Color(m.cfg.Background)
	rowFill := lipgloss.NewStyle().Background(bg)
	scrollbarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.BorderColor)).
		Background(bg)
	contentWidth := chatInnerWidth(m.width)
	textWidth := max(1, contentWidth-1)

	for len(visible) < visibleHeight {
		visible = append(visible, "")
	}
	top, thumbHeight, trackHeight := m.splashScrollbarThumb()
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
			if m.maxSplashScroll() > 0 && trackPosition >= top && trackPosition < top+thumbHeight {
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
	quitStart int
	quitEnd   int
	sendStart int
	sendEnd   int
	pgUpStart int
	pgUpEnd   int
	pgDnStart int
	pgDnEnd   int
	hasSend   bool
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

// renderStatusBar keeps the ordinary shortcuts on the left and anchors the
// send action to the right edge. The same measured layout supplies mouse
// hitboxes, so clickable areas stay aligned with the rendered text.
func (m model) renderStatusBar(usersOpen, splashOpen bool) (string, footerTargets) {
	availableWidth := max(1, m.width-1) // reserve one final cell as a styled blank
	lead := " " + m.status + " • "
	scrollLabel := "PgUp/PgDn"
	scrollTail := ": Scroll • Ctrl+C: "
	if usersOpen || splashOpen {
		// Keep the secondary views' keyboard shortcut wording identical.
		scrollTail = ": Scroll • Esc: close • Ctrl+C: "
	}
	prefix := lead + scrollLabel + scrollTail

	quitText := "Quit"
	if m.quitConfirm {
		quitText = "Sure?"
	}

	targets := footerTargets{}
	includeSend := !usersOpen && !splashOpen
	sendPrefix := "Enter: "
	sendText := "[Send]"
	quitWidth := lipgloss.Width(quitText)
	fullSendWidth := lipgloss.Width(sendPrefix + sendText)
	shortSendWidth := lipgloss.Width(sendText)
	quitLeftPad := 1
	quitRightPad := 1

	// The Enter hint is always the first thing sacrificed.
	if includeSend && lipgloss.Width(prefix)+quitWidth+2+fullSendWidth > availableWidth {
		sendPrefix = ""
	}

	sendWidth := 0
	if includeSend {
		sendWidth = lipgloss.Width(sendPrefix) + shortSendWidth
		// Preserve both clickable actions; relax Quit padding before dropping
		// the Send action at the physical width limit.
		for availableWidth < quitWidth+sendWidth+quitLeftPad+quitRightPad {
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
		if availableWidth < quitWidth+sendWidth {
			includeSend = false
			sendPrefix = ""
			sendWidth = 0
		}
	}

	quitButtonWidth := quitWidth + quitLeftPad + quitRightPad
	maxPrefixWidth := availableWidth - quitButtonWidth - sendWidth
	if maxPrefixWidth < 0 {
		maxPrefixWidth = 0
	}

	pageLabelWidth := lipgloss.Width(scrollLabel)
	if !usersOpen && !splashOpen {
		// Compression order in the main view:
		// 1. Enter: is removed above.
		// 2. The explanatory shortcut tail is clipped from its right edge.
		// 3. Connected is shortened, one character at a time, as the last resort.
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
				// Never show a partial label as though it were clickable.
				prefix = ""
			}
		}
	} else {
		// Keep the existing panel-specific footer behavior.
		pageLabelStart := lipgloss.Width(lead)
		if maxPrefixWidth < pageLabelStart+pageLabelWidth+1 {
			prefix = ""
		} else {
			prefix = truncateANSI(prefix, maxPrefixWidth)
		}
	}

	prefixVisible := strings.TrimRight(prefix, " ")
	prefixWidth := lipgloss.Width(prefixVisible)

	// Highlight PgUp/PgDn and derive the mouse hitboxes from the final
	// compacted prefix so the clickable areas always match the rendered text.
	renderedPrefix := m.styles.status.Render(prefixVisible)
	pageStartByte := strings.Index(prefixVisible, scrollLabel)
	if pageStartByte >= 0 {
		pageStart := lipgloss.Width(prefixVisible[:pageStartByte])
		pageEndByte := pageStartByte + len(scrollLabel)
		renderedPrefix = m.styles.status.Render(prefixVisible[:pageStartByte]) +
			m.styles.accent.Render("PgUp") +
			m.styles.status.Render("/") +
			m.styles.accent.Render("PgDn") +
			m.styles.status.Render(prefixVisible[pageEndByte:])
		targets.hasPageScroll = true
		targets.pgUpStart = pageStart
		targets.pgUpEnd = pageStart + lipgloss.Width("PgUp")
		targets.pgDnStart = pageStart + lipgloss.Width("PgUp/")
		targets.pgDnEnd = targets.pgDnStart + lipgloss.Width("PgDn")
	}

	gapWidth := availableWidth - prefixWidth - quitButtonWidth - sendWidth
	if gapWidth < 0 {
		gapWidth = 0
	}

	targets.quitStart = prefixWidth + quitLeftPad
	targets.quitEnd = targets.quitStart + quitWidth
	var line strings.Builder
	line.WriteString(renderedPrefix)
	if quitLeftPad > 0 {
		line.WriteString(m.styles.status.Render(strings.Repeat(" ", quitLeftPad)))
	}
	line.WriteString(m.styles.accent.Render(quitText))
	if quitRightPad > 0 {
		line.WriteString(m.styles.status.Render(strings.Repeat(" ", quitRightPad)))
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
	prompt := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.AccentColor)).
		Background(lipgloss.Color(m.cfg.Background)).
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
	inputBackground := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Background))
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
		statusText, _ := m.renderStatusBar(usersOpen, splashOpen)
		status := m.styles.status.Render(statusText)
		parts = append(parts, status)
	}

	// -------------------------------------------------------------------------
	// Full application
	// -------------------------------------------------------------------------

	content := m.styles.app.
		Width(m.width).
		Height(m.height).
		Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return appView(content)
}
