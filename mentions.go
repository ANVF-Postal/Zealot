package main

import (
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type mentionSpan struct {
	Start int
	End   int
	Name  string
}

// mentionSpans finds complete @username references from the supplied room
// roster. Matching is case-insensitive, prefers the longest username, and
// requires token boundaries so an email address or a longer unknown name does
// not accidentally notify somebody.
func mentionSpans(text string, usernames []string) []mentionSpan {
	if len(usernames) == 0 || !strings.Contains(text, "@") {
		return nil
	}
	var spans []mentionSpan
	for offset := 0; offset < len(text); {
		if text[offset] != '@' {
			_, size := utf8.DecodeRuneInString(text[offset:])
			if size < 1 {
				break
			}
			offset += size
			continue
		}

		if offset > 0 {
			previous, _ := utf8.DecodeLastRuneInString(text[:offset])
			if mentionWordRune(previous) {
				offset++
				continue
			}
		}

		bestName := ""
		bestEnd := offset
		for _, username := range usernames {
			if username == "" {
				continue
			}
			end := offset + 1 + len(username)
			if end > len(text) || !strings.EqualFold(text[offset+1:end], username) {
				continue
			}
			if end < len(text) {
				next, _ := utf8.DecodeRuneInString(text[end:])
				if mentionWordRune(next) {
					continue
				}
			}
			if len(username) > len(bestName) {
				bestName = username
				bestEnd = end
			}
		}
		if bestName != "" {
			spans = append(spans, mentionSpan{Start: offset, End: bestEnd, Name: bestName})
			offset = bestEnd
		} else {
			offset++
		}
	}
	return spans
}

func mentionWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
}

// extractMentionNames returns canonical room usernames in first-appearance
// order, with duplicates removed case-insensitively.
func extractMentionNames(text string, usernames []string) []string {
	spans := mentionSpans(text, usernames)
	if len(spans) == 0 {
		return nil
	}
	names := make([]string, 0, len(spans))
	seen := make(map[string]struct{}, len(spans))
	for _, span := range spans {
		key := strings.ToLower(span.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, span.Name)
	}
	return names
}

func messageMentionsUser(msg Packet, username string) bool {
	for _, mentioned := range msg.Mentions {
		if strings.EqualFold(mentioned, username) {
			return true
		}
	}
	return false
}

func (m model) messageBackground(msg Packet) color.Color {
	if (msg.Type == "message" || msg.Type == "fortune_result") && messageMentionsUser(msg, m.user.Name) {
		return lipgloss.Color(m.cfg.Theme.MentionBackgroundColor)
	}
	return lipgloss.Color(m.cfg.Theme.Background)
}

func (m model) mentionCandidates() []string {
	if m.mentionUsernames != nil {
		return m.mentionUsernames
	}
	names := make([]string, 0, len(m.onlineUsers)+1)
	if m.user.Name != "" {
		names = append(names, m.user.Name)
	}
	for _, user := range m.onlineUsers {
		names = append(names, user.Name)
	}
	return names
}

func (m *model) refreshMentionUsernames() {
	names := make([]string, 0, len(m.onlineUsers)+1)
	if m.user.Name != "" {
		names = append(names, m.user.Name)
	}
	for _, user := range m.onlineUsers {
		names = append(names, user.Name)
	}
	m.mentionUsernames = names
}

func (m model) mentionStyle(background color.Color, hovered bool) lipgloss.Style {
	foreground := m.cfg.Theme.AccentColor
	if hovered {
		foreground = m.cfg.Theme.HoverTextColor
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(foreground)).
		Background(background).
		Bold(true)
}

// renderMentionBody colors valid recipient names with the Zealot accent while
// preserving normal or rainbow styling for the rest of the message body.
func renderMentionBody(
	text string,
	usernames []string,
	bodyStyle lipgloss.Style,
	mentionStyle lipgloss.Style,
	rainbow bool,
	rainbowPhase float64,
) string {
	return renderMentionBodyAt(text, usernames, bodyStyle, mentionStyle, rainbow, rainbowPhase, 0)
}

func renderMentionBodyAt(
	text string,
	usernames []string,
	bodyStyle lipgloss.Style,
	mentionStyle lipgloss.Style,
	rainbow bool,
	rainbowPhase float64,
	runeOffset int,
) string {
	spans := mentionSpans(text, usernames)
	if len(spans) == 0 {
		if rainbow {
			return rainbowColor(text, rainbowPhase, runeOffset)
		}
		return bodyStyle.Render(text)
	}

	var out strings.Builder
	position := 0
	for _, span := range spans {
		if span.Start > position {
			plain := text[position:span.Start]
			if rainbow {
				out.WriteString(rainbowColor(plain, rainbowPhase, runeOffset))
			} else {
				out.WriteString(bodyStyle.Render(plain))
			}
			runeOffset += utf8.RuneCountInString(plain)
		}
		out.WriteString(mentionStyle.Render(text[span.Start:span.End]))
		runeOffset += utf8.RuneCountInString(text[span.Start:span.End])
		position = span.End
	}
	if position < len(text) {
		plain := text[position:]
		if rainbow {
			out.WriteString(rainbowColor(plain, rainbowPhase, runeOffset))
		} else {
			out.WriteString(bodyStyle.Render(plain))
		}
	}
	return out.String()
}

// styleANSIVisibleMentions applies mention styling to one already-rendered
// textarea row without disturbing its existing ANSI control sequences.
func styleANSIVisibleMentions(line string, usernames []string, style lipgloss.Style) string {
	if !strings.Contains(line, "@") {
		return line
	}
	var visible strings.Builder
	sourceStarts := make(map[int]int)
	sourceEnds := make(map[int]int)
	for pos := 0; pos < len(line); {
		if line[pos] == 0x1b {
			pos = ansiSequenceEnd(line, pos)
			continue
		}
		_, size := utf8.DecodeRuneInString(line[pos:])
		if size < 1 {
			break
		}
		start := visible.Len()
		sourceStarts[start] = pos
		visible.WriteString(line[pos : pos+size])
		sourceEnds[visible.Len()] = pos + size
		pos += size
	}

	spans := mentionSpans(visible.String(), usernames)
	if len(spans) == 0 {
		return line
	}

	var out strings.Builder
	sourcePosition := 0
	for _, span := range spans {
		sourceStart, startOK := sourceStarts[span.Start]
		sourceEnd, endOK := sourceEnds[span.End]
		if !startOK || !endOK || sourceStart < sourcePosition || sourceEnd < sourceStart {
			continue
		}
		out.WriteString(line[sourcePosition:sourceStart])
		out.WriteString(styleANSIChunk(line[sourceStart:sourceEnd], style))
		// Lipgloss resets SGR attributes at the end of the mention. Restore
		// the textarea's active colors before rendering the following text.
		// This also respects selection/cursor styles already present in the row.
		out.WriteString(ansiSGRStateBefore(line, sourceStart))
		sourcePosition = sourceEnd
	}
	out.WriteString(line[sourcePosition:])
	return out.String()
}

// ansiSGRStateBefore replays the original row's SGR controls up to a
// position. Mention styling ends with an SGR reset, so replaying the source
// controls restores the textarea's foreground/background (and selection
// attributes) without hardcoding any active theme colors.
func ansiSGRStateBefore(line string, limit int) string {
	var state strings.Builder
	for pos := 0; pos < limit; {
		if line[pos] != 0x1b {
			pos++
			continue
		}
		end := ansiSequenceEnd(line, pos)
		if end > limit {
			break
		}
		if end > pos+1 && line[pos+1] == '[' && line[end-1] == 'm' {
			state.WriteString(line[pos:end])
		}
		pos = end
	}
	return state.String()
}

func styleANSIChunk(chunk string, style lipgloss.Style) string {
	var out strings.Builder
	chunkStart := 0
	for pos := 0; pos < len(chunk); {
		if chunk[pos] != 0x1b {
			_, size := utf8.DecodeRuneInString(chunk[pos:])
			if size < 1 {
				break
			}
			pos += size
			continue
		}
		if pos > chunkStart {
			out.WriteString(style.Render(chunk[chunkStart:pos]))
		}
		end := ansiSequenceEnd(chunk, pos)
		out.WriteString(chunk[pos:end])
		pos = end
		chunkStart = pos
	}
	if chunkStart < len(chunk) {
		out.WriteString(style.Render(chunk[chunkStart:]))
	}
	return out.String()
}

// insertMention places the selected username at the textarea cursor and keeps
// adjacent text separated into a complete mention token.
func (m *model) insertMention(username string) tea.Cmd {
	if strings.TrimSpace(username) == "" {
		return nil
	}
	if m.input.HasSelection() {
		m.input.DeleteSelection()
	}

	lines := strings.Split(m.input.Value(), "\n")
	row := min(max(m.input.Line(), 0), len(lines)-1)
	column := min(max(m.input.Column(), 0), len([]rune(lines[row])))
	runes := []rune(lines[row])
	insertion := "@" + username
	if column > 0 && !unicode.IsSpace(runes[column-1]) {
		insertion = " " + insertion
	}
	if column >= len(runes) || (!unicode.IsSpace(runes[column]) && mentionWordRune(runes[column])) {
		insertion += " "
	}
	m.input.InsertString(insertion)

	blinkCmd := m.startCursorBlink()
	if !m.connected {
		return blinkCmd
	}
	if !m.isTyping {
		m.isTyping = true
		m.sendTyping(true)
	}
	typingCmd := m.scheduleTypingClear()
	return tea.Batch(blinkCmd, typingCmd)
}

 
// resetMentionTabCompletion discards the original prefix when the draft is
// edited or the cursor is moved by anything other than another Tab press.
func (m *model) resetMentionTabCompletion() {
	m.mentionTabMatches = nil
	m.mentionTabPrefix = ""
	m.mentionTabValue = ""
	m.mentionTabIndex = 0
}

// completeMentionTab replaces a partial @username at the end of the current
// draft. Subsequent Tab presses cycle the original prefix's matches, rather
// than narrowing matches to the first completed username.
func (m *model) completeMentionTab() bool {
	current := m.input.Value()
	if len(m.mentionTabMatches) > 0 && current == m.mentionTabValue {
		m.mentionTabIndex = (m.mentionTabIndex + 1) % len(m.mentionTabMatches)
		m.mentionTabValue = m.mentionTabPrefix + "@" + m.mentionTabMatches[m.mentionTabIndex]
		m.input.SetValue(m.mentionTabValue)
		m.input.MoveToEnd()
		return true
	}
	m.resetMentionTabCompletion()
	if m.input.HasSelection() { return false }
	lines := strings.Split(current, "\n")
	last := lines[len(lines)-1]
	if m.input.Line() != len(lines)-1 || m.input.Column() != utf8.RuneCountInString(last) {
		return false
	}
	at := strings.LastIndexByte(last, '@')
	if at < 0 || at == len(last)-1 { return false }
	if at > 0 {
		previous, _ := utf8.DecodeLastRuneInString(last[:at])
		if mentionWordRune(previous) { return false }
	}
	fragment := last[at+1:]
	for _, r := range fragment {
		if !mentionWordRune(r) { return false }
	}
	matches := make([]string, 0)
	seen := make(map[string]bool)
	for _, username := range m.mentionCandidates() {
		key := strings.ToLower(username)
		if strings.HasPrefix(key, strings.ToLower(fragment)) && !seen[key] {
			seen[key] = true
			matches = append(matches, username)
		}
	}
	if len(matches) == 0 { return false }
	m.mentionTabMatches = matches
	m.mentionTabPrefix = current[:len(current)-len(fragment)-1]
	m.mentionTabIndex = 0
	m.mentionTabValue = m.mentionTabPrefix + "@" + matches[0]
	m.input.SetValue(m.mentionTabValue)
	m.input.MoveToEnd()
	return true
}
