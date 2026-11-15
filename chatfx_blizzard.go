package main

import "math"

// Blizzard wind has two client-local phases: a sustained direction lasting
// 20-120 seconds, and a shorter quiet period lasting 6-35 seconds. Each
// direction reverses the previous one and chooses a fresh peak speed.
// Smooth ramps make the transition continuous, including its integral.
// The wind offset is evaluated once per frame, independent of render FPS.
type blizzardWindPhase struct {
	gustDuration float64
	calmDuration float64
	rampDuration float64
	speed float64
}

func blizzardPhase(index uint64, salt uint64) blizzardWindPhase {
	seed:=chatFXHash(salt+index*0x9e3779b97f4a7c15+0xb17a)
	gust:=20+100*chatFXUnit(seed+1)
	calm:=6+29*chatFXUnit(seed+2)
	ramp:=3+5*chatFXUnit(seed+3)
	peak:=4+48*chatFXUnit(seed+4) // mild through very intense
	if (index%2)==1 { peak=-peak }
	if chatFXHash(salt+0xb17b)%2==1 { peak=-peak }
	return blizzardWindPhase{
		gustDuration:gust, calmDuration:calm, rampDuration:ramp, speed:peak,
	}
}

// The integral of smoothstep(u) = 3u²-2u³, u in [0,1].
func blizzardEaseIntegral(u float64) float64 {
	u=math.Max(0,math.Min(1,u))
	return u*u*u-0.5*u*u*u*u
}

func blizzardWindSample(seconds float64, salt uint64) (offset,speed float64) {
	if seconds<=0 { return 0,0 }
	remaining:=seconds
	for phase:=uint64(0);phase<65536;phase++ {
		p:=blizzardPhase(phase,salt)
		if remaining>=p.gustDuration {
			// Whole gust contributes its plateau displacement and two ramps.
			offset+=p.speed*(p.gustDuration-p.rampDuration)
			remaining-=p.gustDuration
		} else {
			t:=remaining
			ramp:=p.rampDuration
			switch {
			case t<ramp:
				u:=t/ramp
				offset+=p.speed*ramp*blizzardEaseIntegral(u)
				speed=p.speed*u*u*(3-2*u)
			case t<p.gustDuration-ramp:
				offset+=p.speed*(ramp*0.5+t-ramp)
				speed=p.speed
			default:
				u:=(t-(p.gustDuration-ramp))/ramp
				offset+=p.speed*(p.gustDuration-1.5*ramp)
				offset+=p.speed*ramp*(u-blizzardEaseIntegral(u))
				speed=p.speed*(1-u*u*(3-2*u))
			}
			return offset,speed
		}
		if remaining < p.calmDuration { return offset,0 }
		remaining-=p.calmDuration
	}
	return offset,0
}

func blizzardWindOffset(seconds float64, salt uint64) float64 {
	offset,_:=blizzardWindSample(seconds,salt)
	return offset
}

func blizzardWindSpeed(seconds float64, salt uint64) float64 {
	_,speed:=blizzardWindSample(seconds,salt)
	return speed
}

// Blizzard has a dense field of flakes blown strongly sideways, but avoids
// excessive slash glyphs; the character pool favors dots, commas and stars.
// Wind is sampled once per render rather than recomputed per particle.
func (m model) blizzardPositions(width,height int) []map[int]chatFXCell {
	if width<1 || height<1 { return nil }
	rows:=make([]map[int]chatFXCell,height)
	seconds:=(float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	intensity:=0.72+0.28*rainDensityAt(seconds,m.fxSalt^0xb11aa7d)
	// Terminal drawing cost is often ANSI styling, not the math itself.
	// 650 particles caps expensive styling on large SSH terminals.
	count:=max(24,int(float64(min(650,max(48,width*height/4)))*intensity))
	wind,windSpeed:=blizzardWindSample(seconds,m.fxSalt)
	glyphs:=[]rune{'.', '.', ',', ',', '*', '+', '✻', '·'}
	colors:=[]string{"#F6FCFF","#C9E8FF","#94CBE8","#E3F4FF"}
	for i:=0;i<count;i++ {
		seed:=chatFXHash(uint64(i+1)*0x517cc1b727220a95+m.fxSalt)
		glyph:=glyphs[int(chatFXHash(seed+7)%uint64(len(glyphs)))]
		vertical:=2.1+3.2*chatFXUnit(seed+1)
		if glyph=='.' || glyph==',' {vertical*=1.45}
		startX:=chatFXUnit(seed+2)*float64(width)
		startY:=chatFXUnit(seed+3)*float64(height)
		lateral:=wind*(0.87+0.26*chatFXUnit(seed+4))+
			0.8*math.Sin(seconds*(0.7+0.4*chatFXUnit(seed+5))+chatFXUnit(seed+6)*6.28)
		x:=int(math.Floor(math.Mod(math.Mod(startX+lateral,float64(width))+float64(width),float64(width))))
		y:=int(math.Floor(math.Mod(startY+seconds*vertical,float64(height))))
		// Diagonal streaks are rare rather than dominating the screen.
		if math.Abs(windSpeed)>12 && i%29==0 {
			if windSpeed>0 {glyph='\\'} else {glyph='/'}
		}
		if rows[y]==nil {rows[y]=make(map[int]chatFXCell)}
		rows[y][x]=chatFXCell{
			glyph:glyph,
			color:colors[int(chatFXHash(seed+8)%uint64(len(colors)))],
		}
	}
	return rows
}
