package main

import "math"

// Doom-style fire based on filipedeschamps/doom-fire-algorithm (MIT).
// A hot fuel row sends heat upward with lateral drift and cooling. Terminal
// glyph size and a warm ember-like foreground palette represent the heat;
// no solid-colored background cells are used.
var doomFirePalette = [...]string{
	"#7B231B", "#83271A", "#8C2B19", "#942E19", "#9D3218", "#A53617",
	"#AA3817", "#AF3B17", "#B43D17", "#B84017", "#BD4217", "#C24517",
	"#C74717", "#CE4F19", "#D5571B", "#DC5F1C", "#E3671E", "#EA6E20",
	"#F17622", "#F87E23", "#FF8625", "#FF8D29", "#FF942E", "#FF9B32",
	"#FFA336", "#FFAA3A", "#FFB13F", "#FFB843", "#FFBF47", "#FFC44F",
	"#FFC857", "#FFCD5E", "#FFD266", "#FFD66E", "#FFDB76", "#FFDF7D",
	"#FFE485",
}

// Larger glyphs represent dense hot fire. Sparse cooler tips dissolve into
// punctuation-like embers instead of forming an artificial rectangular edge.
func doomFireGlyph(level uint8, variation uint64) rune {
	switch {
	case level >= 32:
		return []rune{'@','#','%','&'}[int(variation%4)]
	case level >= 26:
		return []rune{'#','%','&','@'}[int(variation%4)]
	case level >= 20:
		return []rune{'%','&','*','o'}[int(variation%4)]
	case level >= 14:
		return []rune{'*','+','o','x'}[int(variation%4)]
	case level >= 8:
		return []rune{'+',';',':','*'}[int(variation%4)]
	case level >= 4:
		return []rune{'.',',',';',':'}[int(variation%4)]
	default:
		return []rune{'.',','}[int(variation%2)]
	}
}

type doomFireState struct {
	width, height int
	heat []uint8
	scratch []uint8
	step uint64
}

func newDoomFireState(width, height int, salt uint64) *doomFireState {
	if width<1 || height<1 {return nil}
	// Simulate the *entire* viewport. The visible tip height is determined
	// by natural cooling, never by a fixed 70% clipping plane.
	state:=&doomFireState{
		width:width,height:height,heat:make([]uint8,width*height),
		scratch:make([]uint8,width),
	}
	for x:=0;x<width;x++ {state.heat[(height-1)*width+x]=36}
	for i:=uint64(0);i<uint64(height+16);i++ {
		state.spread(salt,i)
	}
	return state
}

func (f *doomFireState) spread(salt, index uint64) {
	w,h:=f.width,f.height
	if w<1 || h<1 {return}
	for y:=0;y<h-1;y++ {
		// Rebuild this row completely on every propagation, so a heat
		// value cannot remain frozen in a cell that no source reached.
		clear(f.scratch)
		for x:=0;x<w;x++ {
			source:=int(f.heat[(y+1)*w+x])
			if source==0 {continue}
			random:=chatFXHash(salt+index*0x9e3779b97f4a7c15+
				uint64(y+1)*0xbf58476d1ce4e5b9+uint64(x+1)*0x94d049bb133111eb)
			// 0..4 cooling produces a mixture of short flames and high
			// occasional wisps instead of a flat horizontal ceiling.
			decay:=int(random%5)
			destX:=x+1-int((random>>8)%3)
			if destX<0 {destX=0}
			if destX>=w {destX=w-1}
			heat:=source-decay
			if heat>int(f.scratch[destX]) {
				f.scratch[destX]=uint8(heat)
			}
		}
		copy(f.heat[y*w:(y+1)*w],f.scratch)
	}
	// The base stays vigorous; brighter fuel pulses vary per column.
	base:=(h-1)*w
	for x:=0;x<w;x++ {
		r:=chatFXHash(salt+index*0x517cc1b727220a95+uint64(x)*0x9e3779b97f4a7c15)
		f.heat[base+x]=uint8(32+r%5)
	}
}

// The shared ChatFX simulation is 30 Hz; fire propagates at 15 Hz.
func (f *doomFireState) advance(target uint64,salt uint64) {
	if f==nil || target<=f.step {return}
	for tick:=f.step+1;tick<=target;tick++ {
		if tick%2==0 {f.spread(salt,tick/2)}
	}
	f.step=target
}

func (m model) firePositions(width,height int) []map[int]chatFXCell {
	if width<1 || height<1 {return nil}
	rows:=make([]map[int]chatFXCell,height)
	state:=m.fxFire
	if state==nil || state.width!=width || state.height!=height ||
		state.step>m.fxSimSteps {
		state=newDoomFireState(width,height,m.fxSalt)
	}
	state.advance(m.fxSimSteps,m.fxSalt)
	for y:=0;y<height;y++ {
		for x:=0;x<width;x++ {
			level:=state.heat[y*width+x]
			if level==0 {continue}
			if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
			variation:=chatFXHash(m.fxSalt+uint64(x+1)*0x9e3779b97f4a7c15+
				uint64(y+1)*0xbf58476d1ce4e5b9+state.step/4)
			rows[y][x]=chatFXCell{
				glyph:doomFireGlyph(level,variation),
				color:doomFirePalette[level],
			}
		}
	}
	return rows
}

// Matrix streams descend in individual columns. The leading glyph is bright,
// the rest of the stream uses multiple diminishing green intensities. Streams
// wrap naturally and the symbol at each cell evolves without changing speed.
func (m model) matrixPositions(width,height int) []map[int]chatFXCell {
	if width<1 || height<1 { return nil }
	rows:=make([]map[int]chatFXCell,height)
	t:=(float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	alphabet:=[]rune("012345789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	colors:=[]string{"#D7FFDE","#6CFF93","#24DA63","#0FA541","#08652C"}
	// A little over half the columns contain streams. This is dense enough
	// for a code-rain impression but bounded on large terminals.
	for x:=0;x<width;x++ {
		seed:=chatFXHash(uint64(x+1)*0x9e3779b97f4a7c15+m.fxSalt)
		if chatFXUnit(seed+1)>0.61 {continue}
		length:=4+int(chatFXHash(seed+2)%uint64(max(3,min(16,height+3))))
		speed:=3.6+8.2*chatFXUnit(seed+3)
		phase:=chatFXUnit(seed+4)*float64(height+length)
		period:=float64(height+length+7)
		head:=int(math.Floor(math.Mod(phase+t*speed,period)))-length
		for trail:=0;trail<length;trail++ {
			y:=head-trail
			if y<0||y>=height {continue}
			// Head shines, tail gets darker in several steps.
			shade:=min(4,int(float64(trail)/float64(max(1,length))*5))
			if trail==0 {shade=0}
			key:=chatFXHash(seed+uint64(y+1)*0xbf58476d1ce4e5b9+uint64(int(t*9)))
			glyph:=alphabet[int(key%uint64(len(alphabet)))]
			if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
			rows[y][x]=chatFXCell{glyph:glyph,color:colors[shade]}
		}
	}
	return rows
}
