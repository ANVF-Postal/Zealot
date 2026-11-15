package main

import "math"

// Blizzard wind is an analytic integral of its own velocity. Computing
// positions from elapsed simulation time rather than summing render frames
// keeps motion continuous, even when wind reverses or /fxfps changes.
// The main gust reverses smoothly every ~9-14 seconds; a smaller gust
// makes each client's storm feel less mechanical.
func blizzardWindOffset(seconds float64, salt uint64) float64 {
	period := 18.0 + 10.0*chatFXUnit(salt+0xb17a)
	phase := 2*math.Pi*chatFXUnit(salt+0xb17b)
	omega := 2*math.Pi/period
	gustPeriod := period*0.42
	gustOmega := 2*math.Pi/gustPeriod
	gustPhase := 2*math.Pi*chatFXUnit(salt+0xb17c)
	mainSpeed := 11.0+7.0*chatFXUnit(salt+0xb17d)
	return mainSpeed/omega*(math.Cos(phase)-math.Cos(omega*seconds+phase))+
		2.3/gustOmega*(math.Cos(gustPhase)-math.Cos(gustOmega*seconds+gustPhase))
}

// A derivative separate from the render loop is useful for correctly
// angling streaks into the current wind rather than guessing from frame rate.
func blizzardWindSpeed(seconds float64, salt uint64) float64 {
	period := 18.0+10.0*chatFXUnit(salt+0xb17a)
	phase := 2*math.Pi*chatFXUnit(salt+0xb17b)
	mainSpeed := 11.0+7.0*chatFXUnit(salt+0xb17d)
	gustPeriod := period*0.42
	gustPhase := 2*math.Pi*chatFXUnit(salt+0xb17c)
	return mainSpeed*math.Sin(2*math.Pi*seconds/period+phase)+
		2.3*math.Sin(2*math.Pi*seconds/gustPeriod+gustPhase)
}

// Blizzard is a primary effect: dense, fast lateral travel, varying gusts,
// and a shallow diagonal descent. Unlike gentle snow, particles blow far
// across the terminal and even change direction during a long storm.
func (m model) blizzardPositions(width,height int) []map[int]chatFXCell {
	if width < 1 || height < 1 { return nil }
	rows:=make([]map[int]chatFXCell,height)
	seconds:=(float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	// High density, but still gently fluctuating instead of flickering.
	intensity:=0.72+0.28*rainDensityAt(seconds,m.fxSalt^0xb11aa7d)
	count:=max(24,int(float64(min(1000,max(48,width*height/3)))*intensity))
	wind:=blizzardWindOffset(seconds,m.fxSalt)
	windSpeed:=blizzardWindSpeed(seconds,m.fxSalt)
	glyphs:=[]rune{'.','.','*','+','/', '\\', '✻'}
	colors:=[]string{"#F6FCFF","#C9E8FF","#94CBE8","#E3F4FF"}
	for i:=0;i<count;i++ {
		seed:=chatFXHash(uint64(i+1)*0x517cc1b727220a95+m.fxSalt)
		glyph:=glyphs[int(chatFXHash(seed+7)%uint64(len(glyphs)))]
		// Snow is carried sideways much faster than it falls.
		vertical:=2.1+3.2*chatFXUnit(seed+1)
		if glyph=='.' { vertical*=1.45 }
		startX:=chatFXUnit(seed+2)*float64(width)
		startY:=chatFXUnit(seed+3)*float64(height)
		// Small individual variations preserve a shared wind direction.
		lateral:=wind*(0.82+0.36*chatFXUnit(seed+4))+
			0.8*math.Sin(seconds*(0.7+0.4*chatFXUnit(seed+5))+chatFXUnit(seed+6)*6.28)
		xFloat:=startX+lateral
		x:=int(math.Floor(math.Mod(math.Mod(xFloat,float64(width))+float64(width),float64(width))))
		y:=int(math.Floor(math.Mod(startY+seconds*vertical,float64(height))))
		// Wind-facing flakes read as short driven streaks on fast gusts.
		if math.Abs(windSpeed)>7 {
			if glyph=='/' || glyph=='\\' || glyph=='+' {
				if windSpeed>0 { glyph='\\' } else { glyph='/' }
			}
		}
		if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
		rows[y][x]=chatFXCell{
			glyph:glyph,
			color:colors[int(chatFXHash(seed+8)%uint64(len(colors)))],
		}
	}
	return rows
}
