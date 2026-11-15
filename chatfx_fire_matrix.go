package main

import "math"

// Doom-style fire adapted for terminal cells, following the heat-propagation
// algorithm demonstrated by filipedeschamps/doom-fire-algorithm (MIT).
//
// Every cell stores a 0..36 heat index. A hot bottom row is the fuel; heat
// propagates upward with random lateral displacement and cooling. Unlike a
// collection of independently animated flame glyphs, neighboring hot cells
// form coherent, churning shapes. Colored *cell backgrounds* produce the
// original pixel-fire appearance while preserving foreground chat text.
var doomFirePalette = [...]string{
	"#070707", "#1F0707", "#2F0F07", "#470F07", "#571707", "#671F07",
	"#771F07", "#8F2707", "#9F2F07", "#AF3F07", "#BF4707", "#C74707",
	"#DF4F07", "#DF5707", "#DF5707", "#D75F07", "#D75F07", "#D7670F",
	"#CF6F0F", "#CF770F", "#CF7F0F", "#CF8717", "#C78717", "#C78F17",
	"#C7971F", "#BF9F1F", "#BF9F1F", "#BFA727", "#BFA727", "#BFAF2F",
	"#B7AF2F", "#B7B72F", "#B7B737", "#CFCF6F", "#DFDF9F", "#EFEFC7",
	"#FFFFFF",
}

type doomFireState struct {
	width, height int
	heat []uint8
	step uint64 // fixed-clock simulation step, not rendered frame count
}

func newDoomFireState(width, height int, salt uint64) *doomFireState {
	if width<1 || height<1 {return nil}
	// The fire occupies only the lower viewport; the rest remains clear.
	flameHeight:=min(height,max(5,int(math.Ceil(float64(height)*0.70))))
	state:=&doomFireState{width:width,height:flameHeight,heat:make([]uint8,width*flameHeight)}
	for x:=0;x<width;x++ {state.heat[(flameHeight-1)*width+x]=36}
	// Begin with an established burn, not an empty row that climbs up
	// slowly after the command starts.
	for i:=uint64(0);i<uint64(flameHeight+10);i++ {
		state.spread(salt,i)
	}
	return state
}

func (f *doomFireState) spread(salt, index uint64) {
	w,h:=f.width,f.height
	if w<1 || h<1 {return}
	// Read each row below before writing the row above. This in-place
	// update is a single propagation pass, not an allocation per frame.
	for y:=0;y<h-1;y++ {
		for x:=0;x<w;x++ {
			source:=int(f.heat[(y+1)*w+x])
			random:=chatFXHash(salt+index*0x9e3779b97f4a7c15+
				uint64(y+1)*0xbf58476d1ce4e5b9+uint64(x+1)*0x94d049bb133111eb)
			decay:=int(random%3)
			destX:=x+1-int((random>>8)%3)
			if destX<0 {destX=0}
			if destX>=w {destX=w-1}
			heat:=source-decay
			if heat<0 {heat=0}
			f.heat[y*w+destX]=uint8(heat)
		}
	}
	// The bottom remains hot, with slight bubbling rather than a solid
	// white stripe. Fuel is never networked or tied to render FPS.
	base:=(h-1)*w
	for x:=0;x<w;x++ {
		r:=chatFXHash(salt+index*0x517cc1b727220a95+uint64(x)*0x9e3779b97f4a7c15)
		f.heat[base+x]=uint8(32+r%5)
	}
}

// Step at 15 heat propagations/s on the fixed 30 Hz simulation clock.
// Increasing presentation FPS never increases the fire's physical speed.
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
	if state==nil || state.width!=width ||
		state.height!=min(height,max(5,int(math.Ceil(float64(height)*0.70)))) ||
		state.step>m.fxSimSteps {
		// Covers resize before the next timer tick, as well as direct tests
		// that construct a model without using the usual command startup.
		state=newDoomFireState(width,height,m.fxSalt)
	}
	state.advance(m.fxSimSteps,m.fxSalt)
	for y:=0;y<state.height;y++ {
		row:=height-state.height+y
		for x:=0;x<width;x++ {
			level:=state.heat[y*width+x]
			if level<2 {continue}
			if rows[row]==nil {rows[row]=make(map[int]chatFXCell)}
			color:=doomFirePalette[level]
			rows[row][x]=chatFXCell{glyph:' ',color:color,background:color}
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
