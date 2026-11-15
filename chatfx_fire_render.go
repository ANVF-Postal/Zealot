package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// A small terminal palette preserves the warm heat gradient while making
// adjacent flames share ANSI colors. Seven-color runs are substantially
// cheaper to transmit than one escape sequence for every heated cell.
var doomFireTerminalPalette = [...]string{
	"#A53617", "#C74717", "#EA6E20", "#FF8625",
	"#FFAA3A", "#FFCD5E", "#FFE485",
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

// renderDoomFireRows is a faster renderer for active fire without outgoing
// overlays. It avoids a map allocation per row/cell and batches adjacent
// equal-color glyphs into one lipgloss render call. Chat text and mention
// backgrounds are still respected exactly as in the generic FX renderer.
func (m model) renderDoomFireRows(rows []string, width int) {
	if len(rows)==0 || width<1 {return}
	height:=len(rows)
	state:=m.fxFire
	if state==nil || state.width!=width || state.height!=height ||
		state.step>m.fxSimSteps {
		state=newDoomFireState(width,height,m.fxSalt)
	}
	state.advance(m.fxSimSteps,m.fxSalt)
	background:=lipgloss.Color(m.cfg.Theme.Background)
	fill:=lipgloss.NewStyle().Background(background)
	var styles [len(doomFireTerminalPalette)]lipgloss.Style
	for i,color:=range doomFireTerminalPalette {
		styles[i]=lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Background(background)
	}
	for y,original:=range rows {
		plain:=stripANSISequences(original)
		prefix:=lipgloss.Width(strings.TrimRight(plain," "))
		if prefix>=width || strings.Contains(original,m.cfg.Theme.MentionBackgroundColor) {continue}
		var result strings.Builder
		var run strings.Builder
		lastColor:=-1
		blankStart:=prefix
		wrote:=false
		flush:=func() {
			if run.Len()>0 {
				result.WriteString(styles[lastColor].Render(run.String()))
				run.Reset()
			}
		}
		for x:=prefix;x<width;x++ {
			level:=state.heat[y*width+x]
			if level<5 {
				flush()
				lastColor=-1
				continue
			}
			if lastColor==-1 {
				if x>blankStart {
					result.WriteString(fill.Render(strings.Repeat(" ",x-blankStart)))
				}
			}
			shade:=doomFireShade(level)
			if lastColor!=-1 && shade!=lastColor {flush()}
			lastColor=shade
			variation:=chatFXHash(m.fxSalt+uint64(x+1)*0x9e3779b97f4a7c15+
				uint64(y+1)*0xbf58476d1ce4e5b9+state.step/36)
			run.WriteRune(doomFireGlyph(level,variation))
			blankStart=x+1
			wrote=true
		}
		flush()
		if !wrote {continue}
		if blankStart<width {result.WriteString(fill.Render(strings.Repeat(" ",width-blankStart)))}
		head:=""
		if prefix>0 {head=truncateANSI(original,prefix)}
		rows[y]=head+result.String()
	}
}
