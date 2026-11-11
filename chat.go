package main

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// -----------------------------------------------------------------------------
// Message rendering
// -----------------------------------------------------------------------------

func (m model) renderMessageLines() []string {
	if m.renderCacheValid && m.renderCacheWidth == m.width {
		return m.renderedLines
	}

	return m.renderAllMessageLines()
}


 // renderPacketLineCount calculates the exact wrapped height without styling
 // or constructing terminal row strings. It is used for off-screen messages
 // so scroll geometry remains accurate while rendering stays viewport-bound.
func (m model) renderPacketLineCount(msg Packet, width int) int {
	msg = m.displayedMessage(msg)
	if width < 1 {
		return 1
	}
	contentWidth := chatContentWidth(width)
	switch msg.Type {
	case "letter":
		indent := 4
		if contentWidth <= 4 {
			indent = 0
		} else if contentWidth < 14 {
			indent = 2
		}
		bodyWidth := max(1, contentWidth-indent)
		count := 1 // letter heading
		for _, line := range strings.Split(msg.Text, "\n") {
			line = strings.TrimSuffix(line, "\r")
			line = strings.ReplaceAll(line, "\t", "    ")
			count += len(wrapANSIHardLine(line, bodyWidth))
		}
		return count
	case "system":
		text := fmt.Sprintf("[%s] %s", msg.Time.Local().Format("15:04"), msg.Text)
		return max(1, wrapTextLineCount(text, contentWidth-2))
	case "me":
		text := fmt.Sprintf("[%s] * %s %s", msg.Time.Local().Format("15:04"), msg.User.Name, msg.Text)
		return max(1, wrapTextLineCount(text, contentWidth-2))
	case "message":
		if msg.XKeyscore > 0 {
			return m.xkeyscoreLineCount(msg, width)
		}
	default:
		return max(1, len(m.renderPacketBaseLines(msg, width)))
	}
	// Reply markers can add rows at the end of a parent message. These are
	// uncommon, so use the existing renderer to keep their placement exact.
	if len(m.repliesFor(msg.ID)) > 0 {
		return max(1, len(m.renderPacketLines(msg, width)))
	}

	replyBadge := m.renderReplyBadgeForWidth(msg, width)
	if width <= 48 {
		return m.compactMessageLineCount(msg, width, replyBadge)
	}

	prefixWidth := lipgloss.Width("[" + msg.Time.Local().Format("15:04") + "]") +
		1 + lipgloss.Width(msg.User.Name) + lipgloss.Width(replyBadge) + 2
	if prefixWidth >= contentWidth {
		return m.compactMessageLineCount(msg, width, m.renderReplyBadgeForWidth(msg, 48))
	}
	return max(1, wrapTextLineCount(msg.Text, contentWidth-prefixWidth))
}


func (m model) xkeyscoreLineCount(msg Packet, width int) int {
	badge := m.renderReplyBadgeForWidth(msg, width)
	header := msg
	header.Text = ""
	headerLines := 1
	if width <= 48 {
		headerLines = m.compactMessageLineCount(header, width, badge)
	} else {
		prefixWidth := lipgloss.Width("[" + msg.Time.Local().Format("15:04") + "]") +
			1 + lipgloss.Width(msg.User.Name) + lipgloss.Width(badge) + 1 // colon, no body gap
		if prefixWidth >= chatContentWidth(width) {
			headerLines = m.compactMessageLineCount(header, width, m.renderReplyBadgeForWidth(msg, 48))
		}
	}
	logoRows := 7
	switch msg.XKeyscore {
	case 2:
		logoRows = 8
	case 3:
		logoRows = 5 // four logo rows plus the filled tile's bottom row
	}
	return headerLines + logoRows
}

func (m model) compactMessageLineCount(msg Packet, width int, replyBadge string) int {
	contentWidth := chatContentWidth(width)
	nameWidth := contentWidth - 2 - lipgloss.Width(replyBadge)
	if nameWidth < 1 {
		nameWidth = 1
	}
	nameParts := wrapText(msg.User.Name, nameWidth)
	if len(nameParts) == 0 {
		nameParts = []string{""}
	}
	stampWidth := lipgloss.Width("[" + msg.Time.Local().Format("15:04") + "]")
	if len(nameParts) > 1 {
		wrappedCount := wrapTextLineCountVariable(msg.Text, 0, contentWidth-stampWidth-1, contentWidth)
		if wrappedCount < 2 {
			return len(nameParts) + 1
		}
		return len(nameParts) + wrappedCount
	}

	labelWidth := lipgloss.Width(replyBadge) + lipgloss.Width(nameParts[0]+": ")
	firstWidth := contentWidth - labelWidth
	wrappedCount := wrapTextLineCountVariable(msg.Text, firstWidth, contentWidth-stampWidth-1, contentWidth)
	if wrappedCount < 2 {
		return 2 // A one-row body is followed by its timestamp row.
	}
	return wrappedCount
}

func (m model) renderAllMessageLines() []string {
	var lines []string
	for _, message := range m.messages {
		lines = append(lines, m.renderPacketLines(message, m.width)...)
	}
	return lines
}

// padANSIToWidth truncates a styled line safely and paints its remaining
// cells with the supplied background style.

func rainbowTileLine(text string, phase float64, row int) string {
	var out strings.Builder
	for column, ch := range text {
		position := row + column
		hue := math.Mod(phase+float64(position)*0.015, 1)
		if hue < 0 {
			hue++
		}
		red, green, blue := hsvToRGB(hue, 1, 1)
		out.WriteString(fmt.Sprintf("\x1b[38;2;0;0;0;48;2;%d;%d;%dm%c", red, green, blue, ch))
	}
	if out.Len() > 0 {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// rainbowColor applies lolcat-style HSV cycling: each successive rune advances
// by a small fraction of the hue wheel, creating gradual color transitions.
// Each message starts at a random phase.
func rainbowColor(text string, phase float64, offset int) string {
	var out strings.Builder
	position := offset
	for _, ch := range text {
		hue := math.Mod(phase+float64(position)*0.015, 1)
		if hue < 0 {
			hue++
		}
		red, green, blue := hsvToRGB(hue, 1, 1)
		out.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm%c", red, green, blue, ch))
		position++
	}
	if out.Len() > 0 {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// rainbowLogoColor paints each logo glyph with an explicit message background.
// Unlike ordinary rainbow chat text, logo artwork contains negative-space cells
// whose background must remain the configured chat colour.
func rainbowLogoColor(text string, phase float64, offset int, background string) string {
	var out strings.Builder
	position := offset
	for _, ch := range text {
		hue := math.Mod(phase+float64(position)*0.015, 1)
		if hue < 0 {
			hue++
		}
		red, green, blue := hsvToRGB(hue, 1, 1)
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", red, green, blue))).
			Background(lipgloss.Color(background))
		out.WriteString(style.Render(string(ch)))
		position++
	}
	return out.String()
}

func hsvToRGB(h, s, v float64) (int, int, int) {
	i := int(math.Floor(h * 6))
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return int(math.Round(r * 255)), int(math.Round(g * 255)), int(math.Round(b * 255))
}

// rainbowParts colors wrapped body text while carrying the hue position from
// one visual row to the next.
func rainbowParts(parts []string, phase float64) []string {
	colored := make([]string, len(parts))
	offset := 0
	for i, part := range parts {
		colored[i] = rainbowColor(part, phase, offset)
		offset += utf8.RuneCountInString(part) + 1
	}
	return colored
}

func stripANSISequences(line string) string {
	var visible strings.Builder
	for pos := 0; pos < len(line); {
		if line[pos] == 0x1b {
			pos = ansiSequenceEnd(line, pos)
			continue
		}
		_, size := utf8.DecodeRuneInString(line[pos:])
		if size < 1 {
			break
		}
		visible.WriteString(line[pos : pos+size])
		pos += size
	}
	return visible.String()
}

func visibleChatTextWidth(line string) int {
	var visible strings.Builder
	for pos := 0; pos < len(line); {
		if line[pos] == 0x1b {
			pos = ansiSequenceEnd(line, pos)
			continue
		}
		_, size := utf8.DecodeRuneInString(line[pos:])
		if size < 1 {
			break
		}
		visible.WriteString(line[pos : pos+size])
		pos += size
	}
	return lipgloss.Width(strings.TrimRight(visible.String(), " "))
}

// trimANSIEndPadding removes the terminal-width padding from a rendered row
// while preserving ANSI style sequences, including the resets that close the
// body and row background. Keeping padding in place would put a reply marker
// beyond the chat content boundary, where View truncates it behind the scrollbar.
func trimANSIEndPadding(line string) string {
	var out, pendingANSI, pendingSpaces strings.Builder
	for pos := 0; pos < len(line); {
		if line[pos] == 0x1b {
			end := ansiSequenceEnd(line, pos)
			pendingANSI.WriteString(line[pos:end])
			pos = end
			continue
		}
		_, size := utf8.DecodeRuneInString(line[pos:])
		if size < 1 {
			break
		}
		piece := line[pos : pos+size]
		if piece == " " {
			pendingSpaces.WriteString(piece)
		} else {
			out.WriteString(pendingANSI.String())
			pendingANSI.Reset()
			out.WriteString(pendingSpaces.String())
			pendingSpaces.Reset()
			out.WriteString(piece)
		}
		pos += size
	}
	// Preserve trailing SGR resets and other ANSI control sequences, but drop
	// the printable spaces they were painting.
	out.WriteString(pendingANSI.String())
	return out.String()
}

func (m model) renderPacketLines(msg Packet, width int) []string {
	lines := m.renderPacketBaseLines(msg, width)
	if (msg.Type != "message" && msg.Type != "whisper_result") || msg.ID == "" || msg.XKeyscore > 0 {
		return lines
	}
	replies := m.repliesFor(msg.ID)
	if len(replies) == 0 || len(lines) == 0 {
		return lines
	}
	contentWidth := chatContentWidth(width)
	rowFill := lipgloss.NewStyle().Width(contentWidth).Background(m.messageBackground(msg))
	separatorStyle := lipgloss.NewStyle().Background(m.messageBackground(msg))
	replyMarkerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
		Background(m.messageBackground(msg)).
		Bold(true)

	// Compact messages put the timestamp on its own continuation row when
	// the body fits beside the username. Reply markers belong at the end of
	// the message body, not after that timestamp-only row.
	markerRow := len(lines) - 1
	if markerRow > 0 {
		plainLast := strings.TrimSpace(stripANSISequences(lines[markerRow]))
		if plainLast == "["+msg.Time.Local().Format("15:04")+"]" {
			markerRow--
		}
	}

	for n := range replies {
		marker := fmt.Sprintf("[%d]", n+1)
		styled := replyMarkerStyle.Render(marker)
		trimmed := trimANSIEndPadding(lines[markerRow])
		used := visibleChatTextWidth(trimmed)
		if used+1+lipgloss.Width(marker) <= contentWidth {
			lines[markerRow] = renderChatRow(rowFill, trimmed+separatorStyle.Render(" ")+styled, contentWidth)
		} else {
			lines = append(lines, renderChatRow(rowFill, styled, contentWidth))
			markerRow = len(lines) - 1
		}
	}
	return lines
}

func (m *model) rebuildMessageIndexes() {
	messagesByID := make(map[string]Packet, len(m.messages))
	repliesByMessageID := make(map[string][]string)
	for _, packet := range m.messages {
		if (packet.Type != "message" && packet.Type != "whisper_result") || packet.ID == "" {
			continue
		}
		messagesByID[packet.ID] = packet
		if packet.ReplyTo != "" {
			repliesByMessageID[packet.ReplyTo] = append(repliesByMessageID[packet.ReplyTo], packet.ID)
		}
	}
	m.messageByID = messagesByID
	m.replyIDsByMessageID = repliesByMessageID
}

func (m *model) addMessageToIndexes(packet Packet) {
	if m.messageByID == nil || m.replyIDsByMessageID == nil {
		m.rebuildMessageIndexes()
		return
	}
	if packet.Type != "message" || packet.ID == "" {
		return
	}
	m.messageByID[packet.ID] = packet
	if packet.ReplyTo != "" {
		m.replyIDsByMessageID[packet.ReplyTo] = append(m.replyIDsByMessageID[packet.ReplyTo], packet.ID)
	}
}

func (m model) repliesFor(messageID string) []string {
	if messageID == "" {
		return nil
	}
	if m.replyIDsByMessageID != nil {
		return m.replyIDsByMessageID[messageID]
	}
	var replies []string
	for _, packet := range m.messages {
		if (packet.Type == "message" || packet.Type == "whisper_result") && packet.ReplyTo == messageID && packet.ID != "" {
			replies = append(replies, packet.ID)
		}
	}
	return replies
}

func (m model) hasMessageID(messageID string) bool {
	if messageID == "" {
		return false
	}
	if m.messageByID != nil {
		_, exists := m.messageByID[messageID]
		return exists
	}
	for _, packet := range m.messages {
		if packet.ID == messageID && (packet.Type == "message" || packet.Type == "whisper_result") {
			return true
		}
	}
	return false
}

func (m model) renderReplyBadge(msg Packet) string {
	return m.renderReplyBadgeForWidth(msg, 999)
}

func replyBadgeLabelForWidth(width int) string {
	if width <= 48 {
		return "[R]"
	}
	return "[Re]"
}

func (m model) renderReplyBadgeForWidth(msg Packet, width int) string {
	if msg.ReplyTo == "" {
		return ""
	}
	color := m.cfg.Theme.AccentColor
	if !m.hasMessageID(msg.ReplyTo) {
		color = m.cfg.Theme.BorderColor
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Background(m.messageBackground(msg)).
		Bold(true)
	return style.Render(replyBadgeLabelForWidth(width))
}

func (m model) renderPacketBaseLines(msg Packet, width int) []string {
	original := msg
	msg = m.displayedMessage(msg)
	// The final interior column is reserved for the permanent scrollbar.
	// When the side borders disappear at narrow widths, those two columns
	// become available to message content.
	contentWidth := chatContentWidth(width)

	bg := m.messageBackground(msg)
	text := lipgloss.Color(m.cfg.Theme.TextColor)
	rowFill := lipgloss.NewStyle().Width(contentWidth).Background(bg)
	normalBodyStyle := lipgloss.NewStyle().Foreground(text).Background(bg)
	bodyStyle := normalBodyStyle
	if original.Long {
		bodyStyle = bodyStyle.Foreground(lipgloss.Color(m.cfg.Theme.AccentColor))
	} else if original.Spoiler && !m.revealedSpoilers[original.ID] {
		bodyStyle = bodyStyle.Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor))
	}
	hovered := (msg.Type == "message" || msg.Type == "whisper_result") && msg.ID != "" && (msg.ID == m.hoveredMessageID || msg.ID == m.highlightedMessageID)
	if hovered && !isSpecialMessage(original) {
		hoverColor := lipgloss.Color(m.cfg.Theme.HoverTextColor)
		bodyStyle = bodyStyle.Foreground(hoverColor)
	}

	if msg.Type == "whisper_result" {
		// Private messages are shown in the normal chat stream but have
		// a conspicuous label. Only the sender's name uses their user color.
		// The daemon supplies both authoritative sender and recipient names.
		accent := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
			Background(bg).Bold(true)
		senderStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(msg.User.Color)).Background(bg).Bold(true)
		styled := normalBodyStyle.Render("["+msg.Time.Local().Format("15:04")+"] ") +
			accent.Render("[WHISPER]") + normalBodyStyle.Render(" ") +
			senderStyle.Render(terminalSafeText(msg.User.Name)) +
			m.renderReplyBadgeForWidth(msg, width) +
			normalBodyStyle.Render(" → ") +
			accent.Render("@"+terminalSafeText(msg.WhisperTo)) +
			normalBodyStyle.Render(": ") + bodyStyle.Render(terminalSafeText(msg.Text))
		var result []string
		for _, line := range wrapANSIHardLine(styled, max(1, contentWidth-2)) {
			result = append(result, renderChatRow(rowFill, "  "+line, contentWidth))
		}
		return result
	}

	if msg.Type == "fortune_result" {
		prefix := terminalSafeText(msg.User.Name) + "'s Fortune: "
		if msg.FortuneTo != "" {
			prefix = terminalSafeText(msg.FortuneTo) + ", your fortune is: "
		}
		fortuneAccent := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
			Background(bg).
			Bold(true)
		styled := normalBodyStyle.Render("["+msg.Time.Local().Format("15:04")+"] ") +
			fortuneAccent.Render(prefix) + normalBodyStyle.Render(terminalSafeText(msg.Text))
		var result []string
		for _, line := range wrapANSIHardLine(styled, max(1, contentWidth-2)) {
			result = append(result, renderChatRow(rowFill, "  "+line, contentWidth))
		}
		return result
	}

	if msg.Type == "coinflip_result" || msg.Type == "random_result" || msg.Type == "roll_result" {
		label := "[COIN FLIP]"
		action := msg.User.Name + " flipped a coin: "
		outcome := msg.Text
		if msg.Type == "roll_result" {
			label = "[ROLL]"
		}
		if msg.Type == "random_result" {
			label = "[RANDOM]"
			// Random results are daemon-generated as "MIN–MAX: RESULT".
			// Keep the range with the user's coloured action and render only
			// the final number in normal chat text.
			if colon := strings.LastIndex(msg.Text, ": "); colon >= 0 {
				action = msg.User.Name + " rolled " + msg.Text[:colon+2]
				outcome = msg.Text[colon+2:]
			} else {
				action = msg.User.Name + " rolled: "
			}
		}
		userStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.User.Color)).Background(bg)
		styled := normalBodyStyle.Render("["+msg.Time.Local().Format("15:04")+"] ") +
			m.styles.accent.Render(label) + normalBodyStyle.Render(" ")
		if msg.Type == "roll_result" {
			// Highlight both the username and dice notation in the roller's
			// color, but keep " rolled ", the colon and results in normal text.
			styled += userStyle.Render(msg.User.Name) + normalBodyStyle.Render(" rolled ")
			if colon := strings.Index(msg.Text, ":"); colon >= 0 {
				styled += userStyle.Render(msg.Text[:colon]) + normalBodyStyle.Render(msg.Text[colon:])
			} else {
				styled += normalBodyStyle.Render(msg.Text)
			}
		} else {
			styled += userStyle.Render(action) + normalBodyStyle.Render(outcome)
		}
		var result []string
		for _, line := range wrapANSIHardLine(styled, max(1, contentWidth-2)) {
			result = append(result, renderChatRow(rowFill, "  "+line, contentWidth))
		}
		return result
	}

	if msg.Type == "letter" {
		header := fmt.Sprintf("  [%s] sent a letter:", msg.User.Name)
		lines := []string{renderChatRow(rowFill, m.styles.system.Render(header), contentWidth)}
		indent := 4
		if contentWidth <= 4 {
			indent = 0
		} else if contentWidth < 14 {
			indent = 2
		}
		bodyWidth := contentWidth - indent
		if bodyWidth < 1 {
			bodyWidth = 1
		}
		for _, line := range strings.Split(msg.Text, "\n") {
			line = strings.TrimSuffix(line, "\r")
			// Tabs expand to spaces so a terminal tab stop cannot push a row
			// outside the viewport. Literal spaces are otherwise preserved.
			line = strings.ReplaceAll(line, "\t", "    ")
			wrapped := wrapANSIHardLine(line, bodyWidth)
			for _, part := range wrapped {
				lines = append(lines, renderChatRow(rowFill, strings.Repeat(" ", indent)+part+"\x1b[0m", contentWidth))
			}
		}
		return lines
	}

	if msg.Type == "system" {
		systemText := fmt.Sprintf(
			"[%s] %s",
			msg.Time.Local().Format("15:04"),
			msg.Text,
		)
		wrapped := wrapText(systemText, contentWidth-2)
		lines := make([]string, 0, len(wrapped))
		for _, line := range wrapped {
			styled := m.styles.system.Render("  " + line)
			lines = append(lines, renderChatRow(rowFill, styled, contentWidth))
		}
		return lines
	}

	if msg.Type == "me" {
		line := fmt.Sprintf("[%s] * %s %s", msg.Time.Local().Format("15:04"), msg.User.Name, msg.Text)
		wrapped := wrapText(line, contentWidth-2)
		lines := make([]string, 0, len(wrapped))
		for _, part := range wrapped {
			lines = append(lines, renderChatRow(rowFill, m.styles.system.Render("  "+part), contentWidth))
		}
		return lines
	}

	mentionStyle := m.mentionStyle(bg, hovered)
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(bg)
	timeText := msg.Time.Local().Format("15:04")
	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(msg.User.Color)).
		Background(bg).
		Bold(true)


	replyBadge := m.renderReplyBadgeForWidth(msg, width)
	if msg.XKeyscore > 0 {
		var lines []string
		logoIndent := 0
		if width <= 48 {
			header := msg
			header.Text = ""
			lines = renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, mentionStyle, timeStyle, replyBadge, header, timeText, contentWidth)
		} else {
			prefix := timeStyle.Render("[" + timeText + "]") + normalBodyStyle.Render(" ") + nameStyle.Render(msg.User.Name)
			if replyBadge != "" {
				prefix += replyBadge
			}
			prefix += normalBodyStyle.Render(":")
			if lipgloss.Width(prefix) >= contentWidth {
				header := msg
				header.Text = ""
				lines = renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, mentionStyle, timeStyle, m.renderReplyBadgeForWidth(msg, 48), header, timeText, contentWidth)
			} else {
				logoIndent = lipgloss.Width(prefix)
				lines = append(lines, renderChatRow(rowFill, prefix, contentWidth))
			}
		}

		var logo []string
		switch msg.XKeyscore {
		case 2:
			logo = []string{
				" /##   /##",
				"| ##  / ##",
				"|  ##/ ##/",
				" XKEYSCORE",
				"  >##  ## ",
				" /##/\\  ##",
				"| ##  \\ ##",
				"|__/  |__/",
			}
		case 3:
			logo = []string{
				" __  __",
				" \\ \\/ /",
				"  >  <  XKEYSCORE",
				" /_/\\_\\",
			}
		default:
			logo = []string{
				"░██    ░██ ", " ░██  ░██  ", "  ░██░██   ",
				" XKEYSCORE", "  ░██░██   ", " ░██  ░██  ", "░██    ░██ ",
			}
		}

		bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6")).Background(bg)
		shadow := lipgloss.NewStyle().Foreground(lipgloss.Color("#8993A5")).Background(bg)
		blueWord := lipgloss.NewStyle().Foreground(lipgloss.Color("#2589FF")).Background(bg).Bold(true)
		red := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3B3B")).Background(bg)
		blueBlock := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#2589FF"))

		if msg.XKeyscore == 3 {
			// Keep the lettering black; rainbow mode changes only the tile background.
			boxWidth := 22
			indent := strings.Repeat(" ", logoIndent)
			for row, artLine := range logo {
				padded := "  " + artLine + strings.Repeat(" ", max(0, boxWidth-lipgloss.Width(artLine)-2))
				if msg.Rainbow {
					padded = rainbowTileLine(padded, msg.RainbowPhase, row)
					lines = append(lines, renderChatRow(rowFill, indent+padded, contentWidth))
				} else {
					lines = append(lines, renderChatRow(rowFill, indent+blueBlock.Render(padded), contentWidth))
				}
			}
			bottom := strings.Repeat(" ", boxWidth)
			if msg.Rainbow {
				bottom = rainbowTileLine(bottom, msg.RainbowPhase, len(logo))
				lines = append(lines, renderChatRow(rowFill, indent+bottom, contentWidth))
			} else {
				lines = append(lines, renderChatRow(rowFill, indent+blueBlock.Render(bottom), contentWidth))
			}
			return lines
		}

		for row, artLine := range logo {
			var styled strings.Builder
			styled.WriteString(strings.Repeat(" ", logoIndent))
			if msg.Rainbow {
				// Rainbow-colour the artwork while preserving the logo's central
				// XKEYSCORE word in its original variant-specific colour.
				wordStart := strings.Index(artLine, "XKEYSCORE")
				for column, glyph := range []rune(artLine) {
					if wordStart >= 0 && column >= wordStart && column < wordStart+len("XKEYSCORE") {
						if msg.XKeyscore == 2 {
							styled.WriteString(bright.Render(string(glyph)))
						} else {
							styled.WriteString(blueWord.Render(string(glyph)))
						}
						continue
					}
					styled.WriteString(rainbowLogoColor(string(glyph), msg.RainbowPhase, row+column, m.cfg.Theme.Background))
				}
			} else if msg.XKeyscore == 2 {
				for _, glyph := range artLine {
					if glyph == '#' {
						styled.WriteString(red.Render(string(glyph)))
					} else {
						styled.WriteString(bright.Render(string(glyph)))
					}
				}
			} else if strings.Contains(artLine, "XKEYSCORE") {
				leading := len(artLine) - len(strings.TrimLeft(artLine, " "))
				styled.WriteString(strings.Repeat(" ", leading))
				styled.WriteString(blueWord.Render(strings.TrimLeft(artLine, " ")))
			} else {
				for _, glyph := range artLine {
					switch glyph {
					case '█':
						styled.WriteString(bright.Render(string(glyph)))
					case '░':
						styled.WriteString(shadow.Render(string(glyph)))
					default:
						// Explicitly paint negative space with the normal message
						// background so the terminal default cannot show through.
						styled.WriteString(bright.Render(string(glyph)))
					}
				}
			}
			lines = append(lines, renderChatRow(rowFill, styled.String(), contentWidth))
		}
		return lines
	}
	// In narrow terminals, lead with the username and move the timestamp to
	// the next row. This preserves room for readable body text instead of
	// allowing the timestamp + username prefix to consume the entire width.
	if width <= 48 {
		displayMessage := msg
		if hovered {
			displayMessage.Rainbow = false
		}
		return renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, mentionStyle, timeStyle, replyBadge, displayMessage, timeText, contentWidth)
	}

	timestamp := timeStyle.Render("[" + timeText + "]")
	name := nameStyle.Render(msg.User.Name)
	prefix := timestamp + normalBodyStyle.Render(" ") + name
	if replyBadge != "" {
		prefix += replyBadge
	}
	prefix += normalBodyStyle.Render(": ")
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth >= contentWidth {
		displayMessage := msg
		if hovered {
			displayMessage.Rainbow = false
		}
		return renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, mentionStyle, timeStyle, m.renderReplyBadgeForWidth(msg, 48), displayMessage, timeText, contentWidth)
	}

	wrapped := wrapText(msg.Text, contentWidth-prefixWidth)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}

	lines := make([]string, 0, len(wrapped))
	runeOffset := 0
	firstBody := renderMentionBodyAt(wrapped[0], msg.Mentions, bodyStyle, mentionStyle, msg.Rainbow && !hovered, msg.RainbowPhase, runeOffset)
	lines = append(lines, renderChatRow(rowFill, prefix+firstBody, contentWidth))
	runeOffset += utf8.RuneCountInString(wrapped[0])
	indent := strings.Repeat(" ", prefixWidth)
	for _, line := range wrapped[1:] {
		body := renderMentionBodyAt(line, msg.Mentions, bodyStyle, mentionStyle, msg.Rainbow && !hovered, msg.RainbowPhase, runeOffset)
		lines = append(lines, renderChatRow(rowFill, normalBodyStyle.Render(indent)+body, contentWidth))
		runeOffset += utf8.RuneCountInString(line)
	}
	return lines
}

// renderCompactMessage uses a two-row message header on terminals 48 columns
// wide or narrower: "Name: first part" followed by "[HH:MM] continuation".
// Further continuation rows start at the left edge to reclaim horizontal room.
func renderCompactMessage(
	rowFill lipgloss.Style,
	bodyStyle lipgloss.Style,
	separatorStyle lipgloss.Style,
	nameStyle lipgloss.Style,
	mentionStyle lipgloss.Style,
	timeStyle lipgloss.Style,
	replyBadge string,
	msg Packet,
	timeText string,
	contentWidth int,
) []string {
	if contentWidth < 1 {
		contentWidth = 1
	}
	nameWidth := contentWidth - 2 - lipgloss.Width(replyBadge) // reserve the reply label, colon and body gap
	if nameWidth < 1 {
		nameWidth = 1
	}
	nameParts := wrapText(msg.User.Name, nameWidth)
	if len(nameParts) == 0 {
		nameParts = []string{""}
	}
	stamp := "[" + timeText + "]"
	lines := make([]string, 0, 4)
	runeOffset := 0
	renderBody := func(part string) string {
		rendered := renderMentionBodyAt(part, msg.Mentions, bodyStyle, mentionStyle, msg.Rainbow, msg.RainbowPhase, runeOffset)
		runeOffset += utf8.RuneCountInString(part)
		return rendered
	}

	if len(nameParts) > 1 {
		// Preserve very long usernames by hyphen-wrapping them rather than
		// silently chopping the identity off at the viewport edge.
		for _, part := range nameParts[:len(nameParts)-1] {
			lines = append(lines, renderChatRow(rowFill, nameStyle.Render(part), contentWidth))
		}
		lastNameRow := nameStyle.Render(nameParts[len(nameParts)-1]) + replyBadge + separatorStyle.Render(":")
		lines = append(lines, renderChatRow(rowFill, lastNameRow, contentWidth))
		wrapped := wrapTextVariable(msg.Text, 0, contentWidth-lipgloss.Width(stamp)-1, contentWidth)
		if len(wrapped) < 2 {
			lines = append(lines, renderChatRow(rowFill, timeStyle.Render(stamp), contentWidth))
			return lines
		}
		continuation := timeStyle.Render(stamp)
		if wrapped[1] != "" {
			continuation += separatorStyle.Render(" ") + renderBody(wrapped[1])
		}
		lines = append(lines, renderChatRow(rowFill, continuation, contentWidth))
		for _, part := range wrapped[2:] {
			lines = append(lines, renderChatRow(rowFill, renderBody(part), contentWidth))
		}
		return lines
	}

	label := nameParts[0] + ": "
	firstWidth := contentWidth - lipgloss.Width(replyBadge) - lipgloss.Width(label)
	wrapped := wrapTextVariable(msg.Text, firstWidth, contentWidth-lipgloss.Width(stamp)-1, contentWidth)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	firstRow := nameStyle.Render(nameParts[0]) + replyBadge + separatorStyle.Render(":")
	if wrapped[0] != "" {
		firstRow += separatorStyle.Render(" ") + renderBody(wrapped[0])
	}
	lines = append(lines, renderChatRow(rowFill, firstRow, contentWidth))

	if len(wrapped) < 2 {
		lines = append(lines, renderChatRow(rowFill, timeStyle.Render(stamp), contentWidth))
		return lines
	}
	continuation := timeStyle.Render(stamp)
	if wrapped[1] != "" {
		continuation += separatorStyle.Render(" ") + renderBody(wrapped[1])
	}
	lines = append(lines, renderChatRow(rowFill, continuation, contentWidth))
	for _, part := range wrapped[2:] {
		lines = append(lines, renderChatRow(rowFill, renderBody(part), contentWidth))
	}
	return lines
}
