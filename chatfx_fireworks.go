package main

import "math"

// Firework shells and bursts are generated locally from the effect clock.
// A room broadcasts only "fireworks" and the duration, never the positions.
// Paths are analytic so changing /fxfps never changes their physical speed.
var chatFXFireworkPalette = [][]string{
	{"#FF375F", "#FF7692", "#FFB3C0"}, // ruby
	{"#00E6FF", "#63F4FF", "#B6FBFF"}, // cyan
	{"#FFCF00", "#FFE25A", "#FFF5B0"}, // gold
	{"#A94DFF", "#CC87FF", "#EBD2FF"}, // violet
	{"#22FF91", "#84FFBF", "#C4FFDF"}, // emerald
	{"#FF791A", "#FFA55A", "#FFD3A6"}, // orange
}
var chatFXFireworkRainbow = []string{
	"#FF335C", "#FF9E22", "#FFE329", "#40FC81",
	"#26DFFF", "#4A80FF", "#C45CFF",
}

func fireworkColor(seed uint64, spark int, age float64, rainbow bool) string {
	if rainbow {
		return chatFXFireworkRainbow[(spark+int(seed%uint64(len(chatFXFireworkRainbow))))%len(chatFXFireworkRainbow)]
	}
	family := chatFXFireworkPalette[int(seed%uint64(len(chatFXFireworkPalette)))]
	// Different lightness/saturation even in a single-color explosion.
	return family[(spark+int(age*2))%len(family)]
}

func fireworkShellPosition(x0, vx, curve, launchSpeed, elapsed float64, height int) (float64,float64) {
	return x0+vx*elapsed+curve*elapsed*elapsed,
		float64(height-1)-launchSpeed*elapsed+2.6*elapsed*elapsed
}

// fireworkPositions renders rising neutral "o" shells, warm angled trails,
// and distinct radial, willow and crackling explosions. Detonation particles
// use several lifetimes and a slight downward gravitational drift.
func (m model) fireworkPositions(width,height int) []map[int]chatFXCell {
	if width < 1 || height < 1 { return nil }
	positions := make([]map[int]chatFXCell,height)
	t := (float64(m.fxSimSteps)+float64(m.fxSimAccumulator)/float64(chatFXSimulationStep))/30
	emitUntil := float64(chatFXConfettiDefaultSeconds)
	if fx := m.currentChatFX(); !fx.until.IsZero() && !fx.started.IsZero() {
		emitUntil = max(0,fx.until.Sub(fx.started).Seconds())
	}
	set := func(x,y int,glyph rune,color string) {
		if x<0 || x>=width || y<0 || y>=height { return }
		if positions[y]==nil { positions[y]=make(map[int]chatFXCell) }
		positions[y][x]=chatFXCell{glyph:glyph,color:color}
	}
	// A launch roughly every 0.8s; late arrivals will not spawn once
	// the chosen celebration duration has elapsed.
	first := max(0,int(math.Floor((t-7)/0.78)))
	last := min(400,int(math.Ceil(math.Min(t+0.78,emitUntil)/0.78)))
	for i:=first;i<last;i++ {
		launchAt := float64(i)*0.78
		if launchAt>=emitUntil || t<launchAt { continue }
		seed := chatFXHash(uint64(i+1)*0x9e3779b97f4a7c15+m.fxSalt)
		x0 := (0.14+0.72*chatFXUnit(seed+1))*float64(width)
		vx := (chatFXUnit(seed+2)-0.5)*math.Min(12,float64(width)*0.13)
		curve := (chatFXUnit(seed+3)-0.5)*1.5
		launchSpeed := math.Max(8, float64(height)*0.52)*(0.85+0.30*chatFXUnit(seed+4))
		burstAt := 1.10+0.70*chatFXUnit(seed+5)
		elapsed := t-launchAt
		if elapsed<burstAt {
			x,y := fireworkShellPosition(x0,vx,curve,launchSpeed,elapsed,height)
			set(int(math.Round(x)),int(math.Round(y)),'o',"#ECF1F5")
			for tail:=1;tail<=5;tail++ {
				past := elapsed-float64(tail)*0.09
				if past<0 { break }
				px,py := fireworkShellPosition(x0,vx,curve,launchSpeed,past,height)
				glyph := ':'
				if tail>=3 { glyph='.' }
				if dx:=x-px; dx>1.0 { glyph='/' } else if dx< -1.0 { glyph='\\' }
				warm:=[]string{"#FFF1B4","#FFBB45","#FF8617","#AC4E09"}[min(3,(tail-1)/2)]
				set(int(math.Round(px)),int(math.Round(py)),glyph,warm)
			}
			continue
		}
		burstAge := elapsed-burstAt
		if burstAge>4.8 { continue }
		bx,by := fireworkShellPosition(x0,vx,curve,launchSpeed,burstAt,height)
		kind := i%4 // radial, willow, crackle, and peony ring
		rainbow := i%5==0 || i%7==0
		pieces := 24+int(chatFXHash(seed+6)%22)
		for spark:=0;spark<pieces;spark++ {
			sp := chatFXHash(seed+uint64(spark+11)*0xbf58476d1ce4e5b9)
			lifetime := 0.7+2.6*chatFXUnit(sp+1)
			if kind==1 { lifetime += 1.1 } // willow hangs and cascades
			if kind==2 { lifetime *= 0.52 } // crackle burns out quickly
			if burstAge>=lifetime { continue }
			angle := float64(spark)*2*math.Pi/float64(pieces)+0.12*(chatFXUnit(sp+2)-0.5)
			speed := 3.5+7.5*chatFXUnit(sp+3)
			if kind==1 { speed *= 0.68 }
			if kind==3 { speed = 8.5+2*chatFXUnit(sp+4) } // peony ring
			vxSpark,vySpark:=math.Cos(angle)*speed,math.Sin(angle)*speed
			x := bx+vxSpark*burstAge
			y := by+vySpark*burstAge+(1.5+chatFXUnit(sp+5))*burstAge*burstAge
			progress:=burstAge/lifetime
			glyph:='*'
			switch kind {
			case 1:
				if progress>0.55 { glyph='.' } else if progress>0.25 { glyph='+' }
			case 2: // crackle: # > % > x > , > .
				sequence:=[]rune{'#','%','x',',','.'}
				glyph=sequence[min(len(sequence)-1,int(progress*float64(len(sequence))))]
			default:
				if progress>0.8 {glyph='.'} else if progress>0.52 {glyph='+'}
			}
			color:=fireworkColor(seed,spark,burstAge,rainbow)
			ix,iy:=int(math.Round(x)),int(math.Round(y))
			set(ix,iy,glyph,color)
			if progress<0.82 && burstAge>0.13 && spark%3!=0 {
				// A fainter trailing dot behind a subset of bright sparks.
				px:=int(math.Round(x-vxSpark*0.12))
				py:=int(math.Round(y-vySpark*0.12))
				set(px,py,'.',fireworkColor(seed,spark,burstAge+0.4,rainbow))
			}
		}
	}
	return positions
}
