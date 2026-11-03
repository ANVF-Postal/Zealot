package main

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func padANSIToWidth(line string, width int, fill lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	line = truncateANSI(line, width)
	remaining := width - lipgloss.Width(line)
	if remaining > 0 {
		line += fill.Render(strings.Repeat(" ", remaining))
	}
	return line
}

// renderChatRow keeps every rendered chat row inside the chat viewport.
// Width() alone does not clip overlong content, so wrap functions should keep
// rows bounded and this final guard prevents a malformed/oversized row from
// widening the joined TUI layout.
func renderChatRow(rowFill lipgloss.Style, line string, width int) string {
	if width < 1 {
		width = 1
	}
	return rowFill.Render(truncateANSI(line, width))
}

// ansiSequenceEnd returns the byte index immediately after the ANSI escape
// sequence beginning at i. It is deliberately small and dependency-free.
func ansiSequenceEnd(s string, i int) int {
	if i >= len(s) || s[i] != 0x1b {
		return i
	}
	i++
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[': // CSI, commonly SGR color/style sequences
		i++
		for i < len(s) {
			b := s[i]
			i++
			if b >= 0x40 && b <= 0x7e {
				break
			}
		}
	case ']': // OSC, terminated by BEL or ST
		i++
		for i < len(s) {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
			i++
		}
	default:
		i++ // other two-byte escape sequence
	}
	return i
}

// truncateANSI clips a line to its terminal display width without cutting an
// ANSI escape sequence in half.
func truncateANSI(s string, width int) string {
	if width < 1 {
		width = 1
	}
	var out strings.Builder
	used := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			end := ansiSequenceEnd(s, i)
			out.WriteString(s[i:end])
			i = end
			continue
		}

		_, size := utf8.DecodeRuneInString(s[i:])
		if size < 1 {
			break
		}
		piece := s[i : i+size]
		runeWidth := lipgloss.Width(piece)
		if used+runeWidth > width {
			out.WriteString("\x1b[0m")
			break
		}
		out.WriteString(piece)
		used += runeWidth
		i += size
	}
	return out.String()
}

// wrapANSIHardLine wraps terminal output at display-cell boundaries while
// preserving its visible characters and ANSI sequences. It is intended for
// piped letters/terminal output, where preserving literal spaces is more
// important than conventional word wrapping (ASCII art may be distorted).
func wrapANSIHardLine(s string, width int) []string {
	if width < 1 {
		width = 1
	}

	var lines []string
	var current strings.Builder
	used := 0
	activeSGR := ""

	finishLine := func() {
		if activeSGR != "" {
			current.WriteString("\x1b[0m")
		}
		lines = append(lines, current.String())
		current.Reset()
		if activeSGR != "" {
			// Most terminal output (including lolcat) uses a fresh SGR color
			// before its next colored run. Reapply the latest style at a wrap.
			current.WriteString(activeSGR)
		}
		used = 0
	}

	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			end := ansiSequenceEnd(s, i)
			seq := s[i:end]
			current.WriteString(seq)
			if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
				params := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
				reset := params == ""
				for _, param := range strings.Split(params, ";") {
					if param == "0" {
						reset = true
						break
					}
				}
				if reset && (params == "" || params == "0") {
					activeSGR = ""
				} else {
					activeSGR = seq
				}
			}
			i = end
			continue
		}

		_, size := utf8.DecodeRuneInString(s[i:])
		if size < 1 {
			break
		}
		piece := s[i : i+size]
		pieceWidth := lipgloss.Width(piece)
		if used > 0 && used+pieceWidth > width {
			finishLine()
		}
		current.WriteString(piece)
		used += pieceWidth
		i += size
	}

	if current.Len() > 0 || len(lines) == 0 {
		if activeSGR != "" {
			current.WriteString("\x1b[0m")
		}
		lines = append(lines, current.String())
	}
	return lines
}

// rainbowTileLine paints every cell of an XKeyscore tile row with a smooth
// rainbow background while keeping the logo glyphs black.
// rainbowTileLine paints a tile row with a diagonal rainbow background.
// Advancing the phase by one cell per row makes the gradient flow from the
// top-left toward the bottom-right, like lolcat's line-by-line offset.

func wrapText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	return wrapTextVariable(text, width, width, width)
}

// wrapTextVariable wraps prose into rows with a possibly different capacity for
// the first and second row. It prefers spaces between words; only tokens that
// cannot fit on an otherwise empty row are split. URL-like tokens prefer
// punctuation breakpoints and are never given synthetic hyphens.
func wrapTextVariable(text string, firstWidth, secondWidth, fullWidth int) []string {
	if firstWidth < 0 {
		firstWidth = 0
	}
	if secondWidth < 1 {
		secondWidth = 1
	}
	if fullWidth < 1 {
		fullWidth = 1
	}
	lineWidth := func(lineIndex int) int {
		switch lineIndex {
		case 0:
			return firstWidth
		case 1:
			return secondWidth
		default:
			return fullWidth
		}
	}

	var result []string
	current := ""
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			if current != "" {
				result = append(result, current)
				current = ""
			}
			result = append(result, "")
			continue
		}

		for _, token := range words {
			remaining := token
			for remaining != "" {
				capWidth := lineWidth(len(result))
				if capWidth < 1 {
					// A zero-width first row means the message must begin on the
					// next row (for example, when a long username fills row one).
					if current != "" {
						result = append(result, current)
						current = ""
						continue
					}
					result = append(result, "")
					continue
				}

				if current != "" {
					candidate := current + " " + remaining
					if lipgloss.Width(candidate) <= capWidth {
						current = candidate
						remaining = ""
						break
					}

					result = append(result, current)
					current = ""
					continue
				}

				if lipgloss.Width(remaining) <= capWidth {
					current = remaining
					remaining = ""
					break
				}

				part, rest := splitLongWordPrefix(remaining, capWidth, isURLLike(token))
				if part == "" || rest == remaining {
					// Progress even for unusual zero-width/control-only tokens.
					_, size := utf8.DecodeRuneInString(remaining)
					if size < 1 {
						remaining = ""
						break
					}
					part, rest = remaining[:size], remaining[size:]
				}
				result = append(result, part)
				remaining = rest
			}
		}
		if current != "" {
			result = append(result, current)
			current = ""
		}
	}
	if current != "" {
		result = append(result, current)
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}


 // wrapTextLineCountVariable mirrors wrapTextVariable without constructing
 // wrapped output strings. Resize uses it to keep an exact history height
 // index while formatting only the messages near the viewport.
func wrapTextLineCountVariable(text string, firstWidth, secondWidth, fullWidth int) int {
	if firstWidth < 0 {
		firstWidth = 0
	}
	if secondWidth < 1 {
		secondWidth = 1
	}
	if fullWidth < 1 {
		fullWidth = 1
	}
	lineWidth := func(lineIndex int) int {
		switch lineIndex {
		case 0:
			return firstWidth
		case 1:
			return secondWidth
		default:
			return fullWidth
		}
	}

	lineCount := 0
	currentWidth := 0
	currentHasText := false
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			if currentHasText {
				lineCount++
				currentWidth = 0
				currentHasText = false
			}
			lineCount++
			continue
		}

		for _, token := range words {
			remaining := token
			for remaining != "" {
				capacity := lineWidth(lineCount)
				if capacity < 1 {
					if currentWidth > 0 {
						lineCount++
						currentWidth = 0
						continue
					}
					lineCount++
					continue
				}

				remainingWidth := lipgloss.Width(remaining)
				if currentHasText {
					if currentWidth+1+remainingWidth <= capacity {
						currentWidth += 1 + remainingWidth
						remaining = ""
						break
					}
					lineCount++
					currentWidth = 0
					currentHasText = false
					continue
				}

				if remainingWidth <= capacity {
					currentWidth = remainingWidth
					currentHasText = true
					remaining = ""
					break
				}

				part, rest := splitLongWordPrefix(remaining, capacity, isURLLike(token))
				if part == "" || rest == remaining {
					_, size := utf8.DecodeRuneInString(remaining)
					if size < 1 {
						remaining = ""
						break
					}
					part, rest = remaining[:size], remaining[size:]
				}
				lineCount++
				remaining = rest
			}
		}
		if currentHasText {
			lineCount++
			currentWidth = 0
			currentHasText = false
		}
	}
	if lineCount == 0 {
		return 1
	}
	return lineCount
}

func wrapTextLineCount(text string, width int) int {
	if width < 1 {
		width = 1
	}
	return wrapTextLineCountVariable(text, width, width, width)
}

func splitLongWordPrefix(word string, width int, urlLike bool) (string, string) {
	if width < 1 || word == "" {
		return "", word
	}
	used := 0
	lastFit := 0
	lastPunctuation := 0

	for i := 0; i < len(word); {
		if word[i] == 0x1b {
			i = ansiSequenceEnd(word, i)
			lastFit = i
			continue
		}
		r, size := utf8.DecodeRuneInString(word[i:])
		if size < 1 {
			break
		}
		pieceWidth := lipgloss.Width(string(r))
		if used+pieceWidth > width {
			break
		}
		used += pieceWidth
		i += size
		lastFit = i
		if strings.ContainsRune("./\\-_?:&=#%+@,;~!$'()[]{}", r) {
			lastPunctuation = i
		}
	}

	if lastPunctuation > 0 && lastPunctuation < len(word) {
		return word[:lastPunctuation], word[lastPunctuation:]
	}

	// For URLs/domains, don't insert synthetic hyphens: that would alter the
	// visible address when someone copies it from the terminal.
	cut := lastFit
	if !urlLike && width > 1 {
		cut = 0
		used = 0
		for i := 0; i < len(word); {
			if word[i] == 0x1b {
				i = ansiSequenceEnd(word, i)
				cut = i
				continue
			}
			r, size := utf8.DecodeRuneInString(word[i:])
			if size < 1 {
				break
			}
			pieceWidth := lipgloss.Width(string(r))
			if used+pieceWidth > width-1 {
				break
			}
			used += pieceWidth
			i += size
			cut = i
		}
	}
	if cut <= 0 {
		_, size := utf8.DecodeRuneInString(word)
		if size < 1 {
			return "", word
		}
		cut = size
	}
	part := word[:cut]
	rest := word[cut:]
	if !urlLike && width > 1 && part != "" {
		part += "-"
	}
	return part, rest
}

func isURLLike(word string) bool {
	lower := strings.ToLower(word)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ftp://") ||
		strings.HasPrefix(lower, "www.") ||
		strings.Contains(word, ".") ||
		strings.Contains(word, "/") ||
		strings.ContainsAny(word, "?&=#")
}
