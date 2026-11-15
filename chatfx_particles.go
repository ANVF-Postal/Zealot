package main

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// The rain density follows a deterministic, client-local weather cycle.
// It holds each target for 30-180 seconds, then eases over 3-12 seconds
// to the next target. No weather-state animation data travels over IPC.
func rainDensityAt(seconds float64, salt uint64) float64 {
	if seconds < 0 { seconds = 0 }
	// Bound lookup work even in an unusually long-running terminal session.
	seconds = math.Mod(seconds, 3600)
	previous := 0.65
	for segment:=uint64(0); segment<128; segment++ {
		seed := chatFXHash(salt+segment*0x9e3779b97f4a7c15)
		target := 0.28 + 0.70*chatFXUnit(seed+1)
		hold := 30 + 150*chatFXUnit(seed+2)
		transition := 3 + 9*chatFXUnit(seed+3)
		if seconds < hold { return previous }
		seconds -= hold
		if seconds < transition {
			u := seconds/transition
			u = u*u*(3-2*u) // smoothstep: no abrupt change at either end
			return previous+(target-previous)*u
		}
		seconds -= transition
		previous = target
	}
	return previous
}

// Candy shades come in vibrant and pastel variants. Individual confetti
// flakes slowly flip saturation as they fall, independently of their motion.
var chatFXCandy = [][2]string{
	{"#FF5FA2", "#FFC0D9"}, // strawberry
	{"#5CDEAA", "#B6F3DC"}, // mint
	{"#69A6FF", "#C1D9FF"}, // blue raspberry
	{"#B987FF", "#E0CCFF"}, // grape
	{"#FFBC50", "#FFE5A4"}, // lemon
	{"#FF806E", "#FFC4B9"}, // peach
}
var chatFXConfettiGlyphs = []rune{'*', '%', '.', '/', '\\', '+'}

type chatFXCell struct {
	glyph rune
	color string
}

func (m model) confettiPositions(width, height int) []map[int]chatFXCell {
	if width < 1 || height < 1 { return nil }
	positions := make([]map[int]chatFXCell, height)
	count := min(360, max(24, width*height/11))
	t := (float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	for i:=0; i<count; i++ {
		seed := chatFXHash(uint64(i+1)*0x9e3779b97f4a7c15+m.fxSalt)
		speed := 2.8 + 7.2*chatFXUnit(seed+1)
		// Each particle starts above the top border at a staggered time,
		// then respawns after reaching the bottom of the viewport.
		period := float64(height+10)/speed + 3
		start := float64(i)*0.095
		age := t-start
		if age < 0 { continue }
		cycle := math.Floor(age/period)
		age -= cycle*period
		if age < 0 { continue }
		y := int(math.Floor(age*speed - 1))
		if y >= height+7 { continue }
		generation := seed+uint64(cycle)*0x9e3779b97f4a7c15
		x := int(chatFXUnit(generation+4)*float64(width))
		streamer := i%23 == 0 // ~4%: much rarer than ordinary confetti
		if !streamer && i%3 == 0 {
			// No more than two columns of gentle side-to-side sway.
			x += int(math.Round(2*math.Sin(age*(0.8+chatFXUnit(seed+5))+chatFXUnit(seed+6)*6)))
		}
		if x < 0 || x >= width { continue }
		palette := chatFXCandy[int(chatFXHash(generation+7)%uint64(len(chatFXCandy)))]
		shade := int(math.Floor(age*1.8+chatFXUnit(seed+8)*2))%2
		color := palette[shade]
		if streamer {
			length := 3+int(chatFXHash(generation+9)%5) // 3-7 rows
			for tail:=0; tail<length; tail++ {
				row := y-tail
				if row < 0 || row >= height { continue }
				glyph := '\\'
				if tail%2 == 1 { glyph = '/' }
				if positions[row] == nil { positions[row] = make(map[int]chatFXCell) }
				positions[row][x] = chatFXCell{glyph:glyph,color:color}
			}
		} else if y >= 0 {
			glyph := chatFXConfettiGlyphs[int(chatFXHash(generation+10)%uint64(len(chatFXConfettiGlyphs)))]
			if positions[y] == nil { positions[y] = make(map[int]chatFXCell) }
			positions[y][x] = chatFXCell{glyph:glyph,color:color}
		}
	}
	return positions
}

// renderChatFXBackground decorates only blank cells in the visible chat area.
// It leaves message content, scrollbar, borders, typing line and hitboxes alone.
func (m model) renderChatFXBackground(rows []string, width int) {
	fx := m.currentChatFX().name
	if (fx != "rain" && fx != "confetti") || width < 2 || len(rows) == 0 { return }
	positions := make([]map[int]chatFXCell, len(rows))
	if fx == "rain" {
		for y, row := range m.rainPositions(width,len(rows)) {
			if len(row) == 0 { continue }
			positions[y] = make(map[int]chatFXCell,len(row))
			for x, glyph := range row {
				positions[y][x] = chatFXCell{glyph:glyph,color:"#689CC4"}
			}
		}
	} else {
		positions = m.confettiPositions(width,len(rows))
	}
	background := lipgloss.Color(m.cfg.Theme.Background)
	fill := lipgloss.NewStyle().Background(background)
	styles := make(map[string]lipgloss.Style,16)
	for y, original := range rows {
		if len(positions[y]) == 0 { continue }
		plain := stripANSISequences(original)
		prefix := lipgloss.Width(strings.TrimRight(plain," "))
		if prefix >= width { continue }
		// A mention row can have a different background color. Don't erase
		// its styling while laying down particles.
		if strings.Contains(original,m.cfg.Theme.MentionBackgroundColor) { continue }
		var tail strings.Builder
		blankStart := prefix
		for x:=prefix; x<width; x++ {
			cell,exists := positions[y][x]
			if !exists { continue }
			if x>blankStart { tail.WriteString(fill.Render(strings.Repeat(" ",x-blankStart))) }
			style,ok := styles[cell.color]
			if !ok {
				style=lipgloss.NewStyle().Foreground(lipgloss.Color(cell.color)).Background(background)
				styles[cell.color]=style
			}
			tail.WriteString(style.Render(string(cell.glyph)))
			blankStart=x+1
		}
		if blankStart == prefix { continue }
		if blankStart<width { tail.WriteString(fill.Render(strings.Repeat(" ",width-blankStart))) }
		head := ""
		if prefix>0 { head=truncateANSI(original,prefix) }
		rows[y]=head+tail.String()
	}
}
