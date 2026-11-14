package main

import (
	"fmt"
	"math/rand"
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
			m.ensureChatFXTick()
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
			m.ensureChatFXTick()
		} else if m.client != nil && m.connected {
			_ = m.client.send(Packet{Type:"fx_start", Room:m.room, FX:"rain", FXSeconds:seconds})
		}
	case "/stop", "/stopfx":
		if len(fields)==2 && strings.EqualFold(fields[1], "self") {
			m.fxLocal = chatFXState{}
			m.fxLocalOverride = false
			m.chatFXNotice("Local ChatFX stopped.")
			m.ensureChatFXTick()
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
		m.ensureChatFXTick()
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
	if m.fxTickPending || !m.chatFXVisible() || m.currentChatFX().name == "" { return nil }
	m.fxTickPending = true
	m.fxTickGeneration++
	generation := m.fxTickGeneration
	return tea.Tick(chatFXFrameTime, func(time.Time) tea.Msg { return chatFXTickMsg{generation:generation} })
}

func (m *model) advanceChatFX(msg chatFXTickMsg) tea.Cmd {
	if msg.generation != m.fxTickGeneration { return nil }
	m.fxTickPending = false
	if m.fxLocalOverride && !m.fxLocal.until.IsZero() && !time.Now().Before(m.fxLocal.until) {
		m.fxLocalOverride = false
		m.fxLocal = chatFXState{}
	}
	if !m.chatFXVisible() || m.currentChatFX().name == "" { return nil }
	m.fxFrame++
	return m.ensureChatFXTick()
}

// renderChatFXBackground paints only whitespace cells of the chat viewport.
// Never modify the scrollbar, chat borders, typing row or any input hitboxes.
func (m model) renderChatFXBackground(rows []string, width int) {
	if m.currentChatFX().name != "rain" || width < 2 { return }
	color := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#689CC4")).
		Background(lipgloss.Color(m.cfg.Theme.Background))
	base := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Theme.Background))
	frame := int(m.fxFrame)
	salt := int(m.fxSalt % 997)
	for row, original := range rows {
		// Respect every visible glyph (including the styled text already
		// rendered by Zealot). Only draw in the blank suffix of each row.
		plain := stripANSISequences(original)
		prefix := lipgloss.Width(strings.TrimRight(plain, " "))
		if prefix >= width { continue }
		// Skip rows with distinct backgrounds rather than repaint their
		// trailing padding. Such rows can include mention highlighting.
		if strings.Contains(original, m.cfg.Theme.MentionBackgroundColor) { continue }
		var tail strings.Builder
		for col:=prefix; col<width; col++ {
			drop := false
			for particle:=0; particle<max(5,width/6); particle++ {
				x := (particle*47+salt+particle*particle*7)%width
				y := (particle*19+salt/7+frame*(1+particle%3))%max(1,len(rows))
				if x==col && y==row { drop=true; break }
			}
			if drop {
				if rand.Intn(5)==0 { tail.WriteString(color.Render("│")) } else { tail.WriteString(color.Render("·")) }
			} else {
				tail.WriteString(base.Render(" "))
			}
		}
		// truncateANSI preserves the styling and content of the prefix.
		// For a fully empty row, no prefix is needed.
		head := ""
		if prefix > 0 { head=truncateANSI(original, prefix) }
		rows[row] = head+tail.String()
	}
}

func formatChatFXStart(name string, seconds int) string {
	if seconds == 0 { return fmt.Sprintf("%s started for the room.", name) }
	return fmt.Sprintf("%s started for the room (%ds).", name, seconds)
}
