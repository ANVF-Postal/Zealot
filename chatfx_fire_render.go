package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// The original lipgloss.Render call for each flame run dominated the
// 10 ms / 17k-allocation benchmark on a wide terminal. These reusable
// ANSI sequences write terminal colors directly, with no per-run style
// allocations or ANSI parsing. All glyphs remain ordinary ASCII.
var doomFireANSI = [...]string{
	"\x1b[38;2;165;54;23m",
	"\x1b[38;2;199;71;23m",
	"\x1b[38;2;234;110;32m",
	"\x1b[38;2;255;134;37m",
	"\x1b[38;2;255;170;58m",
	"\x1b[38;2;255;205;94m",
	"\x1b[38;2;255;228;133m",
}

func doomFireShade(level uint8) int {
	switch {
	case level < 11: return 0
	case level < 16: return 1
	case level < 21: return 2
	case level < 25: return 3
	case level < 29: return 4
	case level < 33: return 5
	default: return 6
	}
}

// Compute the theme background once per frame. Keeping a concrete color
// ensures the fire does not overwrite the user's theme with terminal black.
func doomFireBackgroundANSI(color string) string {
	if len(color)==7 && color[0]=='#' {
		r,er:=strconv.ParseUint(color[1:3],16,8)
		g,eg:=strconv.ParseUint(color[3:5],16,8)
		b,eb:=strconv.ParseUint(color[5:7],16,8)
		if er==nil && eg==nil && eb==nil {
			return fmt.Sprintf("\x1b[48;2;%d;%d;%dm",r,g,b)
		}
	}
	return "\x1b[49m"
}

// Dense fire is drawn without building a map of particles and without
// invoking lipgloss.Render for every colored character or run.
func (m model) renderDoomFireRows(rows []string, width int) {
	if len(rows)==0 || width<1 {return}
	viewportHeight:=len(rows)
	flameHeight:=doomFireHeight(width,viewportHeight)
	state:=m.fxFire
	if state==nil || state.width!=width || state.height!=flameHeight ||
		state.step>m.fxSimSteps {
		state=newDoomFireState(width,viewportHeight,m.fxSalt)
	}
	state.advance(m.fxSimSteps,m.fxSalt)
	bgANSI:=doomFireBackgroundANSI(m.cfg.Theme.Background)
	blend:=doomFireBlend(state.step,float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))
	// Render the actual heat grid; the mostly empty upper rows are
	// skipped without creating additional particles or ANSI fragments.
	for y:=viewportHeight-flameHeight;y<viewportHeight;y++ {
		rowOffset:=(y-(viewportHeight-flameHeight))*width
		hot:=false
		for x:=0;x<width;x++ {
			if state.heatLevel(rowOffset+x,blend)>=5 {hot=true;break}
		}
		if !hot {continue}
		original:=rows[y]
		// Fast-path ordinary padded rows: avoid stripping ANSI + measuring
		// and do nothing to existing chat text or mention backgrounds.
		plain:=stripANSISequences(original)
		prefix:=lipgloss.Width(strings.TrimRight(plain," "))
		if prefix>=width || strings.Contains(original,m.cfg.Theme.MentionBackgroundColor) {continue}
		var result strings.Builder
		result.Grow(width*2+100)
		lastShade:=-1
		wrote:=false
		for x:=prefix;x<width;x++ {
			level:=state.heatLevel(rowOffset+x,blend)
			if level<5 {
				if lastShade>=0 {result.WriteString("\x1b[39m");lastShade=-1}
				result.WriteByte(' ')
				continue
			}
			if !wrote {
				wrote=true
			}
			shade:=doomFireShade(level)
			if shade!=lastShade {
				result.WriteString(doomFireANSI[shade])
				lastShade=shade
			}
			variation:=chatFXHash(m.fxSalt+uint64(x+1)*0x9e3779b97f4a7c15+
				uint64(y+1)*0xbf58476d1ce4e5b9+state.step/36)
			result.WriteRune(doomFireGlyph(level,variation))
		}
		if !wrote {continue}
		head:=""
		if prefix>0 {head=truncateANSI(original,prefix)}
		rows[y]=head+bgANSI+result.String()+"\x1b[0m"
	}
}
