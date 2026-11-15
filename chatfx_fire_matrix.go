package main

import "math"

// Flame tongues are narrow, connected columns rising from the bottom of the
// chat viewport. Their tips dance independently while the body below them
// remains filled, avoiding the appearance of scattered orange snow.
func (m model) firePositions(width,height int) []map[int]chatFXCell {
	if width<1 || height<1 { return nil }
	rows:=make([]map[int]chatFXCell,height)
	t:=(float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	// Fire hugs the lower portion of the chat pane. Its height slowly
	// breathes, with faster flickers at each independent tongue.
	maxHeight:=max(2,min(height,int(math.Ceil(float64(height)*0.58))))
	colors:=[]string{"#FF4C09","#FF7814","#FF9D20","#FFC53A","#FFE27A"}
	glyphs:=[]rune{'▓','▒','w','v','^','*'}
	for x:=0;x<width;x++ {
		// Adjacent cells share some motion so flames have coherent shapes.
		group:=x/4
		seed:=chatFXHash(uint64(group+1)*0x9e3779b97f4a7c15+m.fxSalt)
		local:=chatFXHash(uint64(x+1)*0xbf58476d1ce4e5b9+m.fxSalt)
		wave:=0.5+0.5*math.Sin(t*(2.0+1.6*chatFXUnit(seed+1))+chatFXUnit(seed+2)*6.28)
		flicker:=0.5+0.5*math.Sin(t*(3.4+1.4*chatFXUnit(local+1))+chatFXUnit(local+2)*6.28)
		breath:=0.5+0.5*math.Sin(t*0.37+chatFXUnit(seed+3)*3)
		tongue:=1+int(float64(maxHeight-1)*(0.13+0.48*wave+0.20*flicker+0.19*breath))
		if tongue>maxHeight { tongue=maxHeight }
		// A few taller tongues flare above the body, especially in larger panes.
		if x%11==3 && wave>0.65 { tongue=min(maxHeight,tongue+2) }
		for depth:=0;depth<tongue;depth++ {
			y:=height-1-depth
			if y<0 { break }
			// Small moving gaps in the tips; keep the bottom connected.
			if depth>3 && depth>tongue-3 &&
				chatFXUnit(chatFXHash(local+uint64(int(t*11)+depth)*17))>0.82 {continue}
			progress:=float64(depth)/float64(max(1,tongue))
			glyph:='▓'
			switch {
			case depth==tongue-1: glyph='^'
			case progress>0.75: glyph='*'
			case progress>0.48: glyph=glyphs[2+int(chatFXHash(local+uint64(depth))%2)]
			case progress>0.22: glyph='▒'
			}
			colorIndex:=min(len(colors)-1,int(progress*float64(len(colors))))
			// Alternate yellow tongues and warm orange tongues.
			if x%7==0 && colorIndex>0 {colorIndex--}
			if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
			rows[y][x]=chatFXCell{glyph:glyph,color:colors[colorIndex]}
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
