package main

import (
	"fmt"
	"math"
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
const (
	chatFXDefaultFPS = 15
	chatFXMaxFPS = 60
	chatFXSimulationStep = time.Second / 30 // fixed 30 Hz, regardless of render FPS
	chatFXMaxSeconds = 300
)

type chatFXTickMsg struct {
	generation int
	at time.Time
}

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
	case "/fxfps":
		if len(fields) != 2 {
			m.chatFXNotice("Usage: /fxfps 1-60")
			return
		}
		fps, err := strconv.Atoi(fields[1])
		if err != nil || fps < 1 || fps > chatFXMaxFPS {
			m.chatFXNotice("Usage: /fxfps 1-60")
			return
		}
		m.fxRenderFPS = fps
		m.chatFXNotice(fmt.Sprintf("ChatFX rendering set to %d FPS (simulation stays at 30 Hz).", fps))
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

// chatFXFrameInterval controls only how often a new frame is presented.
// The physics below always advances in 1/30-second steps.
func (m model) chatFXFrameInterval() time.Duration {
	fps := m.fxRenderFPS
	if fps < 1 || fps > chatFXMaxFPS { fps = chatFXDefaultFPS }
	return time.Second / time.Duration(fps)
}

func (m *model) ensureChatFXTick() tea.Cmd {
	if m.fxTickPending || m.currentChatFX().name == "" { return nil }
	m.fxTickPending = true
	m.fxTickGeneration++
	generation := m.fxTickGeneration
	interval := m.chatFXFrameInterval()
	// Alternate panels never need full-rate ChatFX repaints.
	if !m.chatFXVisible() { interval = time.Second }
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return chatFXTickMsg{generation: generation, at: t}
	})
}

// advanceChatFXClock has no dependency on the presentation frame rate.
// The accumulator advances a fixed-step simulation clock, with a remainder
// used for smooth interpolation on faster displays.
func (m *model) advanceChatFXClock(now time.Time) {
	if now.IsZero() { now = time.Now() }
	if m.fxLastTick.IsZero() {
		m.fxLastTick = now
		return
	}
	delta := now.Sub(m.fxLastTick)
	m.fxLastTick = now
	if delta < 0 { delta = 0 }
	if delta > 2*time.Second { delta = 2*time.Second }
	m.fxSimAccumulator += delta
	steps := m.fxSimAccumulator / chatFXSimulationStep
	m.fxSimSteps += uint64(steps)
	m.fxSimAccumulator -= steps * chatFXSimulationStep
}

func (m *model) advanceChatFX(msg chatFXTickMsg) tea.Cmd {
	if msg.generation != m.fxTickGeneration { return nil }
	m.fxTickPending = false
	if m.fxLocalOverride && !m.fxLocal.until.IsZero() && !time.Now().Before(m.fxLocal.until) {
		m.fxLocalOverride = false
		m.fxLocal = chatFXState{}
	}
	if m.currentChatFX().name == "" {
		m.fxLastTick = time.Time{}
		return nil
	}
	if m.chatFXVisible() {
		m.advanceChatFXClock(msg.at)
		m.fxFrame++
	} else {
		// Don't simulate unseen FX or catch up a long panel pause.
		m.fxLastTick = time.Time{}
	}
	return m.ensureChatFXTick()
}

// chatFXHash produces a reproducible per-client particle distribution without
// global random state or viewport-dependent initialization.
func chatFXHash(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func chatFXUnit(x uint64) float64 {
	return float64(chatFXHash(x)>>11) / float64(uint64(1)<<53)
}

// rainPositions recomputes its distribution for the *current* visible width
// and height on every render, including the very first one. Resizing is never
// required to initialize or extend the effect to the viewport boundaries.
func (m model) rainPositions(width, height int) []map[int]rune {
	if width < 1 || height < 1 { return nil }
	positions := make([]map[int]rune, height)
	count := min(320, max(12, width*height/18))
	simTime := (float64(m.fxSimSteps) +
		float64(m.fxSimAccumulator)/float64(chatFXSimulationStep)) / 30.0
	for i:=0; i<count; i++ {
		seed := uint64(i+1)*0x9e3779b97f4a7c15 + m.fxSalt
		x0 := chatFXUnit(seed)
		y0 := chatFXUnit(seed+1)
		speed := 7 + 9*chatFXUnit(seed+2)
		drift := 0.4 + 0.7*chatFXUnit(seed+3)
		x := int(math.Floor(math.Mod(x0*float64(width)+simTime*drift,float64(width))))
		y := int(math.Floor(math.Mod(y0*float64(height)+simTime*speed,float64(height))))
		if positions[y] == nil { positions[y] = make(map[int]rune) }
		glyph := '·'
		if chatFXHash(seed+4)%3 == 0 { glyph='│' }
		positions[y][x] = glyph
	}
	return positions
}

// renderChatFXBackground paints the whole available chat *background* but only
// replaces blank cells. Text, buttons, the scrollbar, borders, and the typing
// row are left entirely to the existing renderer.
func (m model) renderChatFXBackground(rows []string, width int) {
	if m.currentChatFX().name != "rain" || width < 2 || len(rows) == 0 { return }
	positions := m.rainPositions(width, len(rows))
	color := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#689CC4")).
		Background(lipgloss.Color(m.cfg.Theme.Background))
	base := lipgloss.NewStyle().Background(lipgloss.Color(m.cfg.Theme.Background))
	for row, original := range rows {
		if len(positions[row]) == 0 { continue }
		plain := stripANSISequences(original)
		// Preserve the complete styled message prefix; only the unused
		// whitespace to its right may be decorated.
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

func formatChatFXStart(name string, seconds int) string {
	if seconds == 0 { return fmt.Sprintf("%s started for the room.", name) }
	return fmt.Sprintf("%s started for the room (%ds).", name, seconds)
}
