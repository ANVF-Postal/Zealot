package main

import "math"

// Embers rise from below the chat pane, drift in small warm air currents,
// and gradually dim as they burn out. Variations are client-local and
// derived only from the FX simulation clock, not the rendering FPS.
func (m model) emberPositions(width,height int) []map[int]chatFXCell {
	if width < 1 || height < 1 { return nil }
	rows:=make([]map[int]chatFXCell,height)
	seconds:=(float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	intensity:=rainDensityAt(seconds,m.fxSalt^0xe8be25)
	count:=max(8,int(float64(min(180,max(14,width*height/30)))*(0.65+0.55*intensity)))
	glyphs:=[]rune{'.','.','·','+','*'}
	for i:=0;i<count;i++ {
		seed:=chatFXHash(uint64(i+1)*0x94d049bb133111eb+m.fxSalt)
		speed:=1.5+3.6*chatFXUnit(seed+1)
		if i%9==0 {speed*=1.45} // a few sparks shoot upward faster
		period:=float64(height+10)/speed+4
		age:=math.Mod(seconds+chatFXUnit(seed+2)*period,period)
		// Spawn below the lower boundary. Some particles burn out early;
		// others survive for the whole upward journey.
		startY:=float64(height)+3*chatFXUnit(seed+3)
		y:=int(math.Floor(startY-age*speed))
		if y<0 || y>=height {continue}
		lifetime:=float64(height)*(0.50+0.7*chatFXUnit(seed+4))/speed
		if age>lifetime {continue}
		startX:=chatFXUnit(seed+5)*float64(width)
		sway:=1.0+2.8*chatFXUnit(seed+6)
		drift:=(chatFXUnit(seed+7)-0.5)*0.75*age
		xFloat:=startX+sway*math.Sin(age*(0.5+0.6*chatFXUnit(seed+8))+chatFXUnit(seed+9)*6.28)+drift
		x:=int(math.Floor(math.Mod(math.Mod(xFloat,float64(width))+float64(width),float64(width))))
		progress:=age/lifetime
		glyph:=glyphs[int(chatFXHash(seed+10)%uint64(len(glyphs)))]
		if progress>0.73 {glyph='.'}
		// Bright gold near the start, fading through orange into dark red.
		colors:=[]string{"#FFE485","#FFBF47","#FF8625","#C74717","#873025"}
		colorIndex:=min(len(colors)-1,int(progress*float64(len(colors))))
		if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
		rows[y][x]=chatFXCell{glyph:glyph,color:colors[colorIndex]}
	}
	return rows
}
