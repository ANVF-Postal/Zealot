package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ChatFX deliberately synchronizes intent rather than frames. Only room-wide
// requests cross the Unix socket; each client renders its own particles.
// Future primary effects replace rain; secondary overlays will have a separate
// slot so two competing weather systems never stack.
const chatFXFrameTime = 100 * time.Millisecond
const chatFXMaxSeconds = 300

type chatFXTickMsg struct{ generation int }

type chatFXState struct {
	name string
	until time.Time // zero means until stopped
}

func chatFXDuration(args []string) (int, bool) {
	if len(args) == 0 { return 0, true }
	if len(args) != 1 { return 0, false }
	n, err := strconv.Atoi(args[0])
	return n, err == nil && n >= 1 && n <= chatFXMaxSeconds
}

func (m *model) chatFXCommand(command string, fields []string) {
	switch command {
	case "/disablefx":
		if len(fields) != 1 { m.chatFXNotice("Usage: /disablefx"); return }
		m.fxDisabled = !m.fxDisabled
		if m.fxDisabled {
			m.chatFXNotice("ChatFX disabled locally. Use /disablefx again to enable.")
		} else {
			m.chatFXNotice("ChatFX enabled locally.")
		}
	case "/rain":
		args := fields[1:]
		self := len(args)>0 && strings.EqualFold(args[0], "self")
		if self { args=args[1:] }
		seconds, ok := chatFXDuration(args)
		if !ok { m.chatFXNotice("Usage: /rain [self] [seconds 1-300]"); return }
		until := time.Time{}
		if seconds > 0 { until = time.Now().Add(time.Duration(seconds)*time.Second) }
		if self {
			m.fxLocal = chatFXState{name:"rain", until:until}
			m.fxLocalOverride = true
			m.chatFXNotice("Local rain started. Use /stopfx self to stop.")
		} else if m.client != nil && m.connected {
			_ = m.client.send(Packet{Type:"fx_start", Room:m.room, FX:"rain", FXSeconds:seconds})
		}
	case "/stop", "/stopfx":
		if len(fields)==2 && strings.EqualFold(fields[1], "self") {
			m.fxLocal = chatFXState{}
			m.fxLocalOverride = false
			m.chatFXNotice("Local ChatFX stopped.")
		} else if len(fields)==1 {
			if m.client != nil && m.connected {
				_ = m.client.send(Packet{Type:"fx_stop", Room:m.room})
			}
		} else {
			m.chatFXNotice("Usage: /stopfx [self] (or /stop)")
		}
	}
}

func (m *model) chatFXNotice(text string) {
	m.appendMessage(Packet{Type:"system", Text:text, Time:time.Now().UTC()})
}

func (m *model) receiveChatFX(packet Packet) {
	switch packet.Type {
	case "fx_state":
		if packet.FX == "rain" && (packet.FXUntil.IsZero() || packet.FXUntil.After(time.Now())) {
			m.fxRoom = chatFXState{name:"rain", until:packet.FXUntil}
		} else {
			m.fxRoom = chatFXState{}
		}
	}
}

func (m model) currentChatFX() chatFXState {
	if m.fxDisabled { return chatFXState{} }
	fx := m.fxRoom
	if m.fxLocalOverride { fx=m.fxLocal }
	if !fx.until.IsZero() && !time.Now().Before(fx.until) {
		// Timed local effects fall back to the room-wide effect.
		if m.fxLocalOverride {
			fx=m.fxRoom
		} else {
			return chatFXState{}
		}
	}
	if !fx.until.IsZero() && !time.Now().Before(fx.until) { return chatFXState{} }
	return fx
}

func (m model) chatFXVisible() bool {
	return !m.splashOpen && !m.usersOpen && !m.themesOpen && !m.commandsOpen && m.longReader == nil
}

func (m *model) ensureChatFXTick() tea.Cmd {
	if m.fxTickPending || m.currentChatFX().name == "" { return nil }
	m.fxTickPending = true
	m.fxTickGeneration++
	generation := m.fxTickGeneration
	interval := chatFXFrameTime
	// Keep the effect alive while a panel is open, but poll far less often.
	if !m.chatFXVisible() { interval = time.Second }
	return tea.Tick(interval, func(time.Time) tea.Msg { return chatFXTickMsg{generation:generation} })
}

func (m *model) advanceChatFX(msg chatFXTickMsg) tea.Cmd {
	if msg.generation != m.fxTickGeneration { return nil }
	m.fxTickPending = false
	if m.fxLocalOverride && !m.fxLocal.until.IsZero() && !time.Now().Before(m.fxLocal.until) {
		m.fxLocalOverride = false
		m.fxLocal = chatFXState{}
	}
	if m.currentChatFX().name == "" { return nil }
	if m.chatFXVisible() { m.fxFrame++ }
	return m.ensureChatFXTick()
}

// renderChatFXBackground paints only whitespace cells of the chat viewport.
// Never modify the scrollbar, chat borders, typing row or any input hitboxes.
func (m model) renderChatFXBackground(rows []string, width int) {
	if m.currentChatFX().name != "rain" || width < 2 || len(rows) == 0 { return }
	color := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#689CC4")).
		Background(lipgloss.Color(m.cfg.Theme.Background))
	base := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Theme.Background))
	frame := int(m.fxFrame)
	salt := int(m.fxSalt % 997)
	// Generate client-local particle positions once per frame, not for
	// every cell. A small fixed maximum keeps render cost bounded.
	positions := make([]map[int]rune, len(rows))
	for particle:=0; particle<min(120,max(5,width/6)); particle++ {
		x := (particle*47+salt+particle*particle*7)%width
		y := (particle*19+salt/7+frame*(1+particle%3))%len(rows)
		if positions[y] == nil { positions[y] = make(map[int]rune) }
		glyph := '·'
		if particleGlyph(y,x,frame,salt) { glyph='│' }
		positions[y][x]=glyph
	}
	for row, original := range rows {
		if len(positions[row]) == 0 { continue }
		plain := stripANSISequences(original)
		// The overlay starts strictly after the last non-whitespace cell,
		// so message text, replies, icons, and click targets stay untouched.
		prefix := lipgloss.Width(strings.TrimRight(plain, " "))
		if prefix >= width { continue }
		var tail strings.Builder
		blankStart := prefix
		for col:=prefix; col<width; col++ {
			glyph, exists := positions[row][col]
			if !exists { continue }
			if col > blankStart {
				tail.WriteString(base.Render(strings.Repeat(" ",col-blankStart)))
			}
			tail.WriteString(color.Render(string(glyph)))
			blankStart=col+1
		}
		if blankStart == prefix { continue }
		if blankStart < width {
			tail.WriteString(base.Render(strings.Repeat(" ",width-blankStart)))
		}
		head := ""
		if prefix > 0 { head=truncateANSI(original, prefix) }
		rows[row]=head+tail.String()
	}
}

func particleGlyph(row, col, frame, salt int) bool {
	return (row*13+col*7+frame+salt)%4 == 0
}

func formatChatFXStart(name string, seconds int) string {
	if seconds == 0 { return fmt.Sprintf("%s started for the room.", name) }
	return fmt.Sprintf("%s started for the room (%ds).", name, seconds)
}
