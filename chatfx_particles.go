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
	// Advance deterministically through variable-length segments. Do not
	// wrap at a fixed wall-clock interval: that would cause a visible jump
	// after an hour of continuous weather.
	previous := 0.65
	for segment:=uint64(0); segment<65536; segment++ {
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
	{"#FF1493", "#FF62B2"}, // hot strawberry pink
	{"#00E5A0", "#57FFC4"}, // neon mint
	{"#1683FF", "#55C1FF"}, // electric blue raspberry
	{"#AA36FF", "#D17AFF"}, // vivid grape
	{"#FFCD00", "#FFE04C"}, // lemon candy
	{"#FF572E", "#FF926D"}, // tangerine
}
var chatFXConfettiGlyphs = []rune{'*', '%', '.', '/', '\\', '+'}

type chatFXCell struct {
	glyph rune
	color string
	background string // optional cell background for pixel-based heat FX
}

func (m model) confettiPositions(width, height int) []map[int]chatFXCell {
	if width < 1 || height < 1 { return nil }
	positions := make([]map[int]chatFXCell, height)
	count := min(360, max(24, width*height/11))
	t := (float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	// The event duration controls births, not visibility. After emission
	// ends, the already-airborne particles finish their current descent.
	emitUntil := math.Inf(1)
	if fx := m.currentChatFX(); !fx.until.IsZero() && !fx.started.IsZero() {
		emitUntil = max(0,fx.until.Sub(fx.started).Seconds())
	}
	for i:=0; i<count; i++ {
		seed := chatFXHash(uint64(i+1)*0x9e3779b97f4a7c15+m.fxSalt)
		speed := 2.8 + 7.2*chatFXUnit(seed+1)
		// Each particle starts above the top border at a staggered time,
		// then respawns after reaching the bottom of the viewport.
		period := float64(height+10)/speed + 3
		start := float64(i)*0.055-0.5
		age := t-start
		if age < 0 { continue }
		cycle := math.Floor(age/period)
		if start+cycle*period >= emitUntil { continue }
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
		} else if y >= 0 && y < height {
			// Particles may continue below the viewport before respawning,
			// but only visible rows can be written to positions.
			glyph := chatFXConfettiGlyphs[int(chatFXHash(generation+10)%uint64(len(chatFXConfettiGlyphs)))]
			if positions[y] == nil { positions[y] = make(map[int]chatFXCell) }
			positions[y][x] = chatFXCell{glyph:glyph,color:color}
		}
	}
	return positions
}

// snowflakeSpeed gives small dots a distinctly quicker fall than the
// other glyphs generated from the same particle seed.
func snowflakeSpeed(seed uint64, glyph rune) float64 {
	speed := 1.15+2.2*chatFXUnit(seed+1)
	if glyph == '.' { speed *= 2.0 }
	return speed
}

// snowPositions uses the same slowly varying weather cycle as rain,
// with lighter particle counts and softly swaying descent.
func (m model) snowPositions(width, height int) []map[int]chatFXCell {
	if width < 1 || height < 1 { return nil }
	positions := make([]map[int]chatFXCell,height)
	t := (float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	intensity := rainDensityAt(t,m.fxSalt^0x5eafc0de)
	count := max(4,int(float64(min(220,max(10,width*height/28)))*intensity))
	glyphs := []rune{'*','.','✻','+'}
	for i:=0;i<count;i++ {
		seed := chatFXHash(uint64(i+1)*0x517cc1b727220a95+m.fxSalt)
		// All snow falls a little faster; tiny dots are the quickest,
		// creating subtle depth without turning snowfall into a storm.
		glyph := glyphs[int(chatFXHash(seed+7)%uint64(len(glyphs)))]
		speed := snowflakeSpeed(seed,glyph)
		startX := chatFXUnit(seed+2)*float64(width)
		startY := chatFXUnit(seed+3)*float64(height)
		sway := 0.7+1.3*chatFXUnit(seed+4)
		frequency := 0.35+0.45*chatFXUnit(seed+5)
		horizontal := startX+sway*math.Sin(t*frequency+chatFXUnit(seed+6)*6.28)
		x := int(math.Floor(math.Mod(math.Mod(horizontal,float64(width))+float64(width),float64(width))))
		y := int(math.Floor(math.Mod(startY+t*speed,float64(height))))
		if positions[y]==nil { positions[y]=make(map[int]chatFXCell) }
		shade := []string{"#DDF4FF","#BCE6FF","#F4FDFF"}[int(chatFXHash(seed+8)%3)]
		positions[y][x]=chatFXCell{glyph:glyph,color:shade}
	}
	return positions
}

// renderChatFXBackground decorates the visible chat pane without changing
// message glyphs, scrollbar geometry, borders, typing line or hitboxes.
// Bubbles are the exception to blank-cell-only painting: their translucent
// RGB backgrounds can show behind ordinary message foreground text.
func (m model) renderChatFXBackground(rows []string, width int) {
	if m.fxDisabled { return }
	fx := m.currentChatFX().name
	if width < 2 || len(rows) == 0 || (fx == "" && len(m.fxFading)==0) { return }
	if fx=="interrogation" && len(m.fxFading)==0 {return}
	if fx=="bubbles" && len(m.fxFading)==0 {
		m.renderBubblesRows(rows,width)
		return
	}
	if fx=="fire" && len(m.fxFading)==0 {
		// Dense fire has a dedicated renderer that batches adjacent glyphs
		// into color runs and avoids hundreds of ANSI fragments per frame.
		m.renderDoomFireRows(rows,width)
		return
	}
	// Outgoing particles are drawn first, the current effect on top.
	// Only one effect emits, even while the last one finishes falling.
	positions := m.fadingChatFXPositions(width,len(rows))
	for y,row := range m.chatFXActivePositions(fx,width,len(rows)) {
		if len(row)==0 { continue }
		if positions[y]==nil { positions[y]=make(map[int]chatFXCell,len(row)) }
		for x,cell:=range row { positions[y][x]=cell }
	}
	background := lipgloss.Color(m.cfg.Theme.Background)
	bgANSI := doomFireBackgroundANSI(m.cfg.Theme.Background)
	styles := make(map[string]lipgloss.Style,16)
	// A palette token is rendered only once per frame, while blank runs are
	// written directly. Calling lipgloss.Render for every gap used to dominate
	// both the frame time and allocation count on dense backgrounds.
	tokens := make(map[chatFXCell]string,32)
	for y, original := range rows {
		if len(positions[y]) == 0 { continue }
		plain := stripANSISequences(original)
		prefix := lipgloss.Width(strings.TrimRight(plain," "))
		if prefix >= width { continue }
		// A mention row can have a different background color. Don't erase
		// its styling while laying down particles.
		if strings.Contains(original,m.cfg.Theme.MentionBackgroundColor) { continue }
		var tail strings.Builder
		tail.Grow(width*3)
		blankStart := prefix
		wrote := false
		for x:=prefix; x<width; x++ {
			cell,exists := positions[y][x]
			if !exists { continue }
			if x>blankStart {
				tail.WriteString(bgANSI)
				tail.WriteString(strings.Repeat(" ",x-blankStart))
			}
			rendered,ok := tokens[cell]
			if !ok {
				style,exists := styles[cell.color+"|"+cell.background]
				if !exists {
					style=lipgloss.NewStyle().Foreground(lipgloss.Color(cell.color)).Background(background)
					if cell.background!="" {style=style.Background(lipgloss.Color(cell.background))}
					styles[cell.color+"|"+cell.background]=style
				}
				rendered=style.Render(string(cell.glyph))
				tokens[cell]=rendered
			}
			tail.WriteString(rendered)
			blankStart=x+1
			wrote=true
		}
		if !wrote { continue }
		if blankStart<width {
			tail.WriteString(bgANSI)
			tail.WriteString(strings.Repeat(" ",width-blankStart))
		}
		head := ""
		if prefix>0 { head=truncateANSI(original,prefix) }
		rows[y]=head+tail.String()+"\\x1b[0m"
	}
	// Bubble backgrounds can coexist with the departing particles of the
	// previously active effect; they never replace those glyphs or chat text.
	if fx=="bubbles" {m.renderBubblesRows(rows,width)}
}
