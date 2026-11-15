package main

import (
	"math"
	"time"
)

// Outgoing effects are captured into moving terminal cells rather than kept
// as emitters. This allows short, client-local overlapping exits when a room
// effect is stopped or replaced. Nothing about these particles reaches the
// daemon, and the shared one-primary-effect rule still applies.
type chatFXFadingParticle struct {
	x, y float64
	vx, vy float64
	glyph rune
	color string
	background string
	cutoff float64
}

type chatFXFading struct {
	at time.Time
	lifetime time.Duration
	particles []chatFXFadingParticle
}

func (m model) chatFXActivePositions(name string,width,height int) []map[int]chatFXCell {
	switch name {
	case "rain":
		rows:=m.rainPositions(width,height)
		out:=make([]map[int]chatFXCell,height)
		for y,row:=range rows {
			if len(row)==0 { continue }
			out[y]=make(map[int]chatFXCell,len(row))
			for x,glyph:=range row { out[y][x]=chatFXCell{glyph:glyph,color:"#689CC4"} }
		}
		return out
	case "snow":
		return m.snowPositions(width,height)
	case "blizzard":
		return m.blizzardPositions(width,height)
	case "embers":
		return m.emberPositions(width,height)
	case "fire":
		return m.firePositions(width,height)
	case "matrix":
		return m.matrixPositions(width,height)
	case "confetti":
		return m.confettiPositions(width,height)
	case "fireworks":
		return m.fireworkPositions(width,height)
	}
	return nil
}

func (m *model) retireChatFX(fx chatFXState) {
	if fx.name=="" || m.fxDisabled || !m.chatFXVisible() { return }
	width,height:=chatContentWidth(m.width),max(1,m.messageHeight()-1)
	if width<2 || height<1 { return }
	rows:=m.chatFXActivePositions(fx.name,width,height)
	lifetime:=4*time.Second
	switch fx.name {
	case "snow": lifetime=7*time.Second
	case "blizzard": lifetime=5*time.Second
	case "embers": lifetime=6*time.Second
	case "fire": lifetime=4*time.Second
	case "matrix": lifetime=5*time.Second
	case "confetti": lifetime=8*time.Second
	case "fireworks": lifetime=6*time.Second
	}
	fade:=chatFXFading{at:time.Now(),lifetime:lifetime}
	for y,row:=range rows {
		for x,cell:=range row {
			seed:=chatFXHash(uint64(x+1)*0x9e3779b97f4a7c15+uint64(y+1)*0xbf58476d1ce4e5b9+m.fxSalt)
			vy:=8.0+7*chatFXUnit(seed+1)
			vx:=(chatFXUnit(seed+2)-0.5)*1.6
			switch fx.name {
			case "snow": vy=1.2+2*chatFXUnit(seed+1);if cell.glyph=='.' { vy*=2.0 };vx*=1.6
			case "blizzard": vy=2.5+3*chatFXUnit(seed+1);vx=blizzardWindSpeed(float64(m.fxSimSteps)/30,m.fxSalt)*(0.65+0.35*chatFXUnit(seed+2))
			case "embers": vy=-(1.5+3.6*chatFXUnit(seed+1));vx*=2.2
			case "fire": vy=-(3+5*chatFXUnit(seed+1));vx*=0.45
			case "matrix": vy=4+7*chatFXUnit(seed+1);vx=0
			case "confetti": vy=3+7*chatFXUnit(seed+1);vx*=2
			case "fireworks": vy=2+4*chatFXUnit(seed+1);vx*=4
			}
			fade.particles=append(fade.particles,chatFXFadingParticle{
				x:float64(x),y:float64(y),vx:vx,vy:vy,glyph:cell.glyph,color:cell.color,background:cell.background,
				cutoff:0.65+0.35*chatFXUnit(seed+3),
			})
		}
	}
	if len(fade.particles)==0 { return }
	m.fxFading=append(m.fxFading,fade)
	if len(m.fxFading)>3 { m.fxFading=m.fxFading[len(m.fxFading)-3:] }
}

func (m *model) trimChatFXFading(now time.Time) {
	if len(m.fxFading)==0 { return }
	keep:=m.fxFading[:0]
	for _,fade:=range m.fxFading {
		if now.Sub(fade.at)<fade.lifetime { keep=append(keep,fade) }
	}
	m.fxFading=keep
}

func (m model) fadingChatFXPositions(width,height int) []map[int]chatFXCell {
	positions:=make([]map[int]chatFXCell,height)
	now:=time.Now()
	for _,fade:=range m.fxFading {
		elapsed:=now.Sub(fade.at).Seconds()
		if elapsed<0 || elapsed>=fade.lifetime.Seconds() {continue}
		progress:=elapsed/fade.lifetime.Seconds()
		for _,particle:=range fade.particles {
			if progress>particle.cutoff { continue }
			x:=int(math.Round(particle.x+particle.vx*elapsed))
			y:=int(math.Round(particle.y+particle.vy*elapsed))
			if x<0||x>=width||y<0||y>=height {continue}
			if positions[y]==nil { positions[y]=make(map[int]chatFXCell) }
			positions[y][x]=chatFXCell{glyph:particle.glyph,color:particle.color,background:particle.background}
		}
	}
	return positions
}

func (m *model) finishExpiredPrimary(now time.Time) {
	// Weather stops emitting at its deadline and drains naturally.
	// Confetti and fireworks already have analytic emission deadlines.
	if m.fxLocalOverride && m.fxLocal.name!="" &&
		m.fxLocal.name!="confetti" && m.fxLocal.name!="fireworks" &&
		!m.fxLocal.until.IsZero() && !now.Before(m.fxLocal.until) {
		m.retireChatFX(m.fxLocal)
		m.fxLocal=chatFXState{}
		m.fxLocalOverride=false
	}
	if m.fxRoom.name!="" && m.fxRoom.name!="confetti" && m.fxRoom.name!="fireworks" &&
		!m.fxRoom.until.IsZero() && !now.Before(m.fxRoom.until) {
		if !m.fxLocalOverride { m.retireChatFX(m.fxRoom) }
		m.fxRoom=chatFXState{}
	}
}
