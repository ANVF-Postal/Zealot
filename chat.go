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

func (m model) renderPacketLines(msg Packet, width int) []string {
	lines := m.renderPacketBaseLines(msg, width)
	if msg.Type != "message" || msg.ID == "" || msg.XKeyscore > 0 { return lines }
	replies := m.repliesFor(msg.ID)
	if len(replies) == 0 || len(lines) == 0 { return lines }
	contentWidth := chatContentWidth(width)
	rowFill := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Background))
	for n := range replies {
		marker := fmt.Sprintf("[%d]", n+1)
		styled := m.styles.accent.Render(marker)
		last := len(lines)-1
		used := lipgloss.Width(lines[last])
		if used+1+lipgloss.Width(marker) <= contentWidth {
			if reset := strings.LastIndex(lines[last], "\x1b[0m"); reset >= 0 {
				lines[last] = lines[last][:reset] + " " + styled + lines[last][reset:]
			} else { lines[last] += " " + styled }
		} else { lines = append(lines, renderChatRow(rowFill, styled, contentWidth)) }
	}
	return lines
}

func (m model) repliesFor(messageID string) []string {
	if messageID == "" {
		return nil
	}
	var replies []string
	for _, packet := range m.messages {
		if packet.Type == "message" && packet.ReplyTo == messageID && packet.ID != "" {
			replies = append(replies, packet.ID)
		}
	}
	return replies
}

func (m model) hasMessageID(messageID string) bool {
	if messageID == "" {
		return false
	}
	for _, packet := range m.messages {
		if packet.ID == messageID && packet.Type == "message" {
			return true
		}
	}
	return false
}

func (m model) renderReplyBadge(msg Packet) string {
	if msg.ReplyTo == "" {
		return ""
	}
	color := m.cfg.AccentColor
	if !m.hasMessageID(msg.ReplyTo) {
		color = m.cfg.BorderColor
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Background(lipgloss.Color(m.cfg.Background)).
		Bold(true)
	return style.Render("[Replied]") + " "
}

func (m model) renderPacketBaseLines(msg Packet, width int) []string {
	// The final interior column is reserved for the permanent scrollbar.
	// When the side borders disappear at narrow widths, those two columns
	// become available to message content.
	contentWidth := chatContentWidth(width)

	bg := lipgloss.Color(m.cfg.Background)
	text := lipgloss.Color(m.cfg.TextColor)
	rowFill := lipgloss.NewStyle().Width(contentWidth).Background(bg)
	normalBodyStyle := lipgloss.NewStyle().Foreground(text).Background(bg)
	bodyStyle := normalBodyStyle
	hovered := msg.Type == "message" && msg.ID != "" && (msg.ID == m.hoveredMessageID || msg.ID == m.highlightedMessageID)
	if hovered {
		hoverColor := lipgloss.Color("#F2D66B")
		bodyStyle = bodyStyle.Foreground(hoverColor)
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

	timeText := msg.Time.Local().Format("15:04")
	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(msg.User.Color)).
		Background(bg).
		Bold(true)

	timeStyle := m.styles.time

	replyBadge := m.renderReplyBadge(msg)
	if msg.XKeyscore > 0 {
		var lines []string
		logoIndent := 0
		if width <= 48 {
			header := msg
			header.Text = ""
			lines = renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, timeStyle, replyBadge, header, timeText, contentWidth)
		} else {
			prefix := replyBadge + m.styles.time.Render("[" + timeText + "]") + normalBodyStyle.Render(" ") + nameStyle.Render(msg.User.Name) + normalBodyStyle.Render(":")
			if lipgloss.Width(prefix) >= contentWidth {
				header := msg
				header.Text = ""
				lines = renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, m.styles.time, replyBadge, header, timeText, contentWidth)
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
					styled.WriteString(rainbowLogoColor(string(glyph), msg.RainbowPhase, row+column, m.cfg.Background))
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
		return renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, timeStyle, replyBadge, displayMessage, timeText, contentWidth)
	}

	timestamp := timeStyle.Render("[" + timeText + "]")
	name := nameStyle.Render(msg.User.Name)
	badgeSegment := ""
	if replyBadge != "" { badgeSegment = " " + strings.TrimSpace(replyBadge) }
	prefix := timestamp + normalBodyStyle.Render(" ") + name + normalBodyStyle.Render(badgeSegment+": ")
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth >= contentWidth {
		displayMessage := msg
		if hovered {
			displayMessage.Rainbow = false
		}
		return renderCompactMessage(rowFill, bodyStyle, normalBodyStyle, nameStyle, m.styles.time, replyBadge, displayMessage, timeText, contentWidth)
	}

	wrapped := wrapText(msg.Text, contentWidth-prefixWidth)
	if msg.Rainbow && !hovered {
		wrapped = rainbowParts(wrapped, msg.RainbowPhase)
	}
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}

	lines := make([]string, 0, len(wrapped))
	lines = append(lines, renderChatRow(rowFill, prefix+bodyStyle.Render(wrapped[0]), contentWidth))
	indent := strings.Repeat(" ", prefixWidth)
	for _, line := range wrapped[1:] {
		lines = append(lines, renderChatRow(rowFill, normalBodyStyle.Render(indent)+bodyStyle.Render(line), contentWidth))
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

	if len(nameParts) > 1 {
		// Preserve very long usernames by hyphen-wrapping them rather than
		// silently chopping the identity off at the viewport edge.
		for i, part := range nameParts[:len(nameParts)-1] {
			line := nameStyle.Render(part)
			if i == 0 {
				line = replyBadge + line
			}
			lines = append(lines, renderChatRow(rowFill, line, contentWidth))
		}
		lastNameRow := nameStyle.Render(nameParts[len(nameParts)-1] + ":")
		lines = append(lines, renderChatRow(rowFill, lastNameRow, contentWidth))
		wrapped := wrapTextVariable(msg.Text, 0, contentWidth-lipgloss.Width(stamp)-1, contentWidth)
		if msg.Rainbow {
			wrapped = rainbowParts(wrapped, msg.RainbowPhase)
		}
		if len(wrapped) < 2 {
			lines = append(lines, renderChatRow(rowFill, timeStyle.Render(stamp), contentWidth))
			return lines
		}
		continuation := timeStyle.Render(stamp)
		if wrapped[1] != "" {
			continuation += separatorStyle.Render(" ") + bodyStyle.Render(wrapped[1])
		}
		lines = append(lines, renderChatRow(rowFill, continuation, contentWidth))
		for _, part := range wrapped[2:] {
			lines = append(lines, renderChatRow(rowFill, bodyStyle.Render(part), contentWidth))
		}
		return lines
	}

	label := nameParts[0] + ": "
	firstWidth := contentWidth - lipgloss.Width(replyBadge) - lipgloss.Width(label)
	wrapped := wrapTextVariable(msg.Text, firstWidth, contentWidth-lipgloss.Width(stamp)-1, contentWidth)
	if msg.Rainbow {
		wrapped = rainbowParts(wrapped, msg.RainbowPhase)
	}
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	firstRow := replyBadge + nameStyle.Render(nameParts[0] + ":")
	if wrapped[0] != "" {
		firstRow += separatorStyle.Render(" ") + bodyStyle.Render(wrapped[0])
	}
	lines = append(lines, renderChatRow(rowFill, firstRow, contentWidth))

	if len(wrapped) < 2 {
		lines = append(lines, renderChatRow(rowFill, timeStyle.Render(stamp), contentWidth))
		return lines
	}
	continuation := timeStyle.Render(stamp)
	if wrapped[1] != "" {
		continuation += separatorStyle.Render(" ") + bodyStyle.Render(wrapped[1])
	}
	lines = append(lines, renderChatRow(rowFill, continuation, contentWidth))
	for _, part := range wrapped[2:] {
		lines = append(lines, renderChatRow(rowFill, bodyStyle.Render(part), contentWidth))
	}
	return lines
}
