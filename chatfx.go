package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
	chatFXConfettiDefaultSeconds = 10
)

type chatFXTickMsg struct {
	generation int
	at time.Time
}

type chatFXState struct {
	name string
	started time.Time // client-local effect timeline origin
	until time.Time // zero means until stopped; confetti drains after this point
}

func chatFXDuration(args []string) (int, bool) {
	if len(args) == 0 { return 0, true }
	if len(args) != 1 { return 0, false }
	n, err := strconv.Atoi(args[0])
	return n, err == nil && n >= 1 && n <= chatFXMaxSeconds
}

// Only these commands may control the FX system. Dispatch them before any
// chat/whisper message handling so they can never leak as ordinary messages.
func isChatFXCommand(command string) bool {
	switch command {
	case "/rain", "/snow", "/blizzard", "/embers", "/fire", "/matrix", "/confetti", "/fireworks", "/stop", "/stopfx", "/disablefx", "/fxfps":
		return true
	}
	return false
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
		// Invalidate any already-scheduled frame so the new interval takes
		// effect immediately (rather than after an old low-FPS timer).
		m.fxTickGeneration++
		m.fxTickPending = false
		m.chatFXNotice(fmt.Sprintf("ChatFX rendering set to %d FPS (simulation stays at 30 Hz).", fps))
	case "/disablefx":
		if len(fields) != 1 { m.chatFXNotice("Usage: /disablefx"); return }
		m.fxDisabled = !m.fxDisabled
		if m.fxDisabled {
			m.chatFXNotice("ChatFX disabled locally. Use /disablefx again to enable.")
		} else {
			m.chatFXNotice("ChatFX enabled locally.")
		}
	case "/rain", "/snow", "/blizzard", "/embers", "/confetti", "/fireworks":
		args := fields[1:]
		self := len(args)>0 && strings.EqualFold(args[0], "self")
		if self { args=args[1:] }
		seconds, ok := chatFXDuration(args)
		if !ok { m.chatFXNotice("Usage: "+command+" [self] [seconds 1-300]"); return }
		name := strings.TrimPrefix(command, "/")
		// Confetti is a celebration, unlike persistent room weather.
		if (name == "confetti" || name == "fireworks") && seconds == 0 { seconds = chatFXConfettiDefaultSeconds }
		until := time.Time{}
		if seconds > 0 { until = time.Now().Add(time.Duration(seconds)*time.Second) }
		if self {
			m.retireChatFX(m.currentChatFX())
			m.fxLocal = chatFXState{name:name, started:time.Now(), until:until}
			m.fxLocalOverride = true
			m.resetChatFXSimulation()
			m.chatFXNotice("Local "+name+" started. Use /stopfx self to stop.")
		} else if m.client != nil && m.connected {
			_ = m.client.send(Packet{Type:"fx_start", Room:m.room, FX:name, FXSeconds:seconds})
		}
	case "/stop", "/stopfx":
		if len(fields)==2 && strings.EqualFold(fields[1], "self") {
			if m.fxLocalOverride { m.retireChatFX(m.fxLocal) }
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
		previous := m.currentChatFX()
		// Preserve the visible room effect as outgoing particles before
		// replacing/stopping its authoritative state.
		if previous.name != "" && !m.fxLocalOverride {
			m.retireChatFX(previous)
		}
		if (packet.FX == "rain" || packet.FX == "snow" || packet.FX == "blizzard" || packet.FX == "embers" || packet.FX == "fire" || packet.FX == "matrix" || packet.FX == "confetti" || packet.FX == "fireworks") &&
			(packet.FXUntil.IsZero() || packet.FXUntil.After(time.Now())) {
			m.fxRoom = chatFXState{name:packet.FX, started:time.Now(), until:packet.FXUntil}
			if !m.fxLocalOverride { m.resetChatFXSimulation() }
		} else {
			m.fxRoom = chatFXState{}
		}
	}
}

// A timed celebration stops *emitting* at its deadline; existing pieces
// keep falling until the slowest one has cleared the viewport.
func (m model) chatFXAlive(fx chatFXState) bool {
	if fx.name == "" { return false }
	if fx.until.IsZero() { return true }
	deadline := fx.until
	if fx.name == "confetti" {
		height := max(1, m.messageHeight()-1)
		// Slowest confetti is 2.8 rows/s, plus a few rows above the top.
		deadline = deadline.Add(time.Duration(math.Ceil(float64(height+14)/2.8)+4)*time.Second)
	}
	if fx.name == "fireworks" { deadline = deadline.Add(8*time.Second) }
	return time.Now().Before(deadline)
}

func (m model) currentChatFX() chatFXState {
	if m.fxDisabled { return chatFXState{} }
	if m.fxLocalOverride && m.chatFXAlive(m.fxLocal) {
		return m.fxLocal
	}
	if m.chatFXAlive(m.fxRoom) { return m.fxRoom }
	return chatFXState{}
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
	if m.fxTickPending || (m.currentChatFX().name == "" && len(m.fxFading)==0) { return nil }
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

// resetChatFXSimulation begins a fresh locally generated effect without
// changing the room's authoritative state.
func (m *model) resetChatFXSimulation() {
	m.fxSimSteps = 0
	m.fxSimAccumulator = 0
	m.fxLastTick = time.Time{}
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
	m.trimChatFXFading(time.Now())
	m.finishExpiredPrimary(time.Now())
	if m.fxLocalOverride && !m.chatFXAlive(m.fxLocal) {
		m.fxLocalOverride = false
		m.fxLocal = chatFXState{}
	}
	if m.currentChatFX().name == "" && len(m.fxFading)==0 {
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
	count := int(float64(min(320, max(12, width*height/18))) * rainDensityAt(
		(float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30, m.fxSalt))
	count = max(1, count)
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

func formatChatFXStart(name string, seconds int) string {
	if seconds == 0 { return fmt.Sprintf("%s started for the room.", name) }
	return fmt.Sprintf("%s started for the room (%ds).", name, seconds)
}
