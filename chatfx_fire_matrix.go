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

// Larger glyphs represent warm, connected flames. Only the occasional cool
// wisp becomes punctuation so the effect doesn't feel like a spark shower.
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

// The heat grid reaches the entire chat area. Visible flames still burn out
// well below its top because each propagation step loses heat. An unbounded
// simulation ceiling is cheaper than hiding a second, artificial cutoff:
// a 320x45 viewport is only 14,400 single-byte heat cells.
func doomFireHeight(width, viewportHeight int) int {
	if width < 1 || viewportHeight < 1 { return 0 }
	return viewportHeight
}

type doomFireState struct {
	width, height int
	heat []uint8
	previous []uint8 // heat before the most recent 7.5 Hz propagation
	scratch []uint8
	step uint64
}

func newDoomFireState(width, height int, salt uint64) *doomFireState {
	if width<1 || height<1 {return nil}
	// Allocate one byte per viewport cell; empty upper rows are skipped
	// by the ANSI renderer, not simulated as independent particles.
	flameHeight:=doomFireHeight(width,height)
	state:=&doomFireState{
		width:width,height:flameHeight,heat:make([]uint8,width*flameHeight),
		previous:make([]uint8,width*flameHeight),scratch:make([]uint8,width),
	}
	for x:=0;x<width;x++ {state.heat[(flameHeight-1)*width+x]=doomFireFuel(salt,0,x)}
	for i:=uint64(0);i<uint64(flameHeight+16);i++ {
		state.spread(salt,i)
	}
	copy(state.previous,state.heat)
	return state
}

// Give neighboring columns similar but not identical fuel strengths.
// Long-lived patches make the flame front uneven without isolated sparks.
func doomFireFuel(salt, index uint64, x int) uint8 {
	patch:=x/5
	phase:=index/12
	a:=chatFXHash(salt+uint64(patch+1)*0x9e3779b97f4a7c15+phase*0xbf58476d1ce4e5b9)
	b:=chatFXHash(salt+uint64(patch+2)*0x9e3779b97f4a7c15+phase*0xbf58476d1ce4e5b9)
	blend:=float64(x%5)/5
	strength:=(1-blend)*float64(a%15)+blend*float64(b%15)
	return uint8(22+int(strength))
}

// fireWind returns a slowly reversing current. All cells in a frame share
// it, so flames bend together instead of jittering independently.
func fireWind(index uint64, salt uint64) float64 {
	phase:=2*math.Pi*chatFXUnit(salt+0xf1e5)
	// Let each direction hold for longer; one complete sway lasts about
	// 33 seconds at the 7.5 Hz heat-step rate.
	return math.Sin(float64(index)*0.025+phase)
}

func (f *doomFireState) spread(salt, index uint64) {
	w,h:=f.width,f.height
	if w<1 || h<1 {return}
	wind:=fireWind(index,salt)
	for y:=0;y<h-1;y++ {
		clear(f.scratch)
		depth:=h-1-y
		for x:=0;x<w;x++ {
			source:=int(f.heat[(y+1)*w+x])
			if source==0 {continue}
			random:=chatFXHash(salt+index*0x9e3779b97f4a7c15+
				uint64(y+1)*0xbf58476d1ce4e5b9+uint64(x+1)*0x94d049bb133111eb)
			// Classic Doom-style cooling with a low, continuous loss.
			// Varying fuel strength creates tall tongues beside shorter ones;
			// no hard height threshold and no artificial ceiling cooling.
			decay:=int(random%3) // 0..2 heat near the fuel
			if depth>2 {decay=1+int((random>>12)%2)} // 1..2 higher up
			if source<17 {decay++} // fine tips burn out instead of hanging in place
			heat:=source-decay
			if heat<=0 {continue}
			// Carry heat downwind during a gust, but leave a slightly
			// weaker vertical feed to keep the base connected and substantial.
			destX:=x
			if math.Abs(wind)>0.16 && int((random>>8)%8)<int(2+4*math.Abs(wind)) {
				if wind>0 {destX++} else {destX--}
				if destX<0 || destX>=w {destX=x}
			}
			if uint8(heat)>f.scratch[destX] {f.scratch[destX]=uint8(heat)}
			if destX!=x && (random>>16)%3==0 {
				verticalHeat:=heat-3
				if verticalHeat>0 && uint8(verticalHeat)>f.scratch[x] {
					f.scratch[x]=uint8(verticalHeat)
				}
			}
		}
		copy(f.heat[y*w:(y+1)*w],f.scratch)
	}
	base:=(h-1)*w
	for x:=0;x<w;x++ {
		f.heat[base+x]=doomFireFuel(salt,index,x)
	}
}

// The shared ChatFX simulation is 30 Hz; fire propagates at 7.5 Hz.
// This keeps the burn calm without slowing other visual effects.
func (f *doomFireState) advance(target uint64,salt uint64) {
	if f==nil || target<=f.step {return}
	for tick:=f.step+1;tick<=target;tick++ {
		if tick%4==0 {
			copy(f.previous,f.heat)
			f.spread(salt,tick/4)
		}
	}
	f.step=target
}

// doomFireBlend blends two consecutive Doom heat states without increasing
// the propagation speed. Phase 0 displays the previous field; the following
// 30 Hz steps (and fractional /fxfps frames) transition toward the new field.
// The next propagation begins at exactly the previous target, avoiding jumps.
func (f *doomFireState) heatLevel(i int, fraction float64) uint8 {
	if f.step<4 || len(f.previous)!=len(f.heat) {
		return f.heat[i]
	}
	phase:=(float64(f.step%4)+fraction)/4
	if phase<0 {phase=0}
	if phase>1 {phase=1}
	old:=float64(f.previous[i])
	current:=float64(f.heat[i])
	return uint8(math.Round(old+(current-old)*phase))
}

func (m model) firePositions(width,height int) []map[int]chatFXCell {
	if width<1 || height<1 {return nil}
	rows:=make([]map[int]chatFXCell,height)
	state:=m.fxFire
	if state==nil || state.width!=width || state.height!=doomFireHeight(width,height) ||
		state.step>m.fxSimSteps {
		state=newDoomFireState(width,height,m.fxSalt)
	}
	state.advance(m.fxSimSteps,m.fxSalt)
	fraction:=float64(m.fxSimAccumulator)/float64(chatFXSimulationStep)
	for y:=0;y<state.height;y++ {
		row:=height-state.height+y
		for x:=0;x<width;x++ {
			level:=state.heatLevel(y*width+x,fraction)
			// Skip the weakest dying heat: fine punctuation should be rare
			// near flame tips, not a constant cloud across the viewport.
			if level<5 {continue}
			if rows[row]==nil {rows[row]=make(map[int]chatFXCell)}
			variation:=chatFXHash(m.fxSalt+uint64(x+1)*0x9e3779b97f4a7c15+
				uint64(y+1)*0xbf58476d1ce4e5b9+state.step/36)
			rows[row][x]=chatFXCell{
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
