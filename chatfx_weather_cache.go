package main

import (
    "sync"
)

// The weather cursors retain the last completed deterministic phase.
// They reset for backward seeks, so tests and resizes can request frames in
// any order without depending on presentation FPS.
type rainPhaseCursor struct {
    at float64
    consumed float64
    segment uint64
    previous float64
    value float64
    valid bool
}
var rainPhaseCursors = struct {
    sync.Mutex
    bySalt map[uint64]rainPhaseCursor
}{bySalt:make(map[uint64]rainPhaseCursor)}

func cachedRainDensity(seconds float64,salt uint64) float64 {
    if seconds<0 {seconds=0}
    rainPhaseCursors.Lock()
    defer rainPhaseCursors.Unlock()
    c:=rainPhaseCursors.bySalt[salt]
    if c.valid && c.at==seconds {return c.value}
    if !c.valid || seconds<c.at {
        c=rainPhaseCursor{previous:0.65}
    }
    remaining:=seconds-c.consumed
    for segment:=c.segment;segment<65536;segment++ {
        seed:=chatFXHash(salt+segment*0x9e3779b97f4a7c15)
        target:=0.28+0.70*chatFXUnit(seed+1)
        hold:=30+150*chatFXUnit(seed+2)
        transition:=3+9*chatFXUnit(seed+3)
        if remaining<hold {
            c.at,c.value,c.valid=seconds,c.previous,true
            rainPhaseCursors.bySalt[salt]=c
            return c.value
        }
        remaining-=hold
        if remaining<transition {
            u:=remaining/transition
            u=u*u*(3-2*u)
            c.at,c.value,c.valid=seconds,c.previous+(target-c.previous)*u,true
            rainPhaseCursors.bySalt[salt]=c
            return c.value
        }
        remaining-=transition
        c.consumed+=hold+transition
        c.previous=target
        c.segment=segment+1
    }
    c.at,c.value,c.valid=seconds,c.previous,true
    rainPhaseCursors.bySalt[salt]=c
    return c.value
}

type blizzardPhaseCursor struct {
    at float64
    consumed float64
    phase uint64
    offset float64
    valueOffset float64
    valueSpeed float64
    valid bool
}
var blizzardPhaseCursors = struct {
    sync.Mutex
    bySalt map[uint64]blizzardPhaseCursor
}{bySalt:make(map[uint64]blizzardPhaseCursor)}

func cachedBlizzardWind(seconds float64,salt uint64) (float64,float64) {
    if seconds<=0 {return 0,0}
    blizzardPhaseCursors.Lock()
    defer blizzardPhaseCursors.Unlock()
    c:=blizzardPhaseCursors.bySalt[salt]
    if c.valid && c.at==seconds {return c.valueOffset,c.valueSpeed}
    if !c.valid || seconds<c.at {c=blizzardPhaseCursor{}}
    remaining:=seconds-c.consumed
    for phase:=c.phase;phase<65536;phase++ {
        p:=blizzardPhase(phase,salt)
        previousOffset:=c.offset
        if remaining>=p.gustDuration {
            c.offset+=p.speed*(p.gustDuration-p.rampDuration)
            remaining-=p.gustDuration
        } else {
            t:=remaining
            ramp:=p.rampDuration
            offset:=c.offset
            speed:=0.0
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
            c.at,c.valueOffset,c.valueSpeed,c.valid=seconds,offset,speed,true
            blizzardPhaseCursors.bySalt[salt]=c
            return offset,speed
        }
        if remaining<p.calmDuration {
            // The gust is finished, but do not commit it to the cursor
            // until its calm period has elapsed. Otherwise next frame
            // would add the same gust displacement a second time.
            completedOffset:=c.offset
            c.offset=previousOffset
            c.at,c.valueOffset,c.valueSpeed,c.valid=seconds,completedOffset,0,true
            blizzardPhaseCursors.bySalt[salt]=c
            return c.offset,0
        }
        remaining-=p.calmDuration
        c.consumed+=p.gustDuration+p.calmDuration
        c.phase=phase+1
    }
    c.at,c.valueOffset,c.valueSpeed,c.valid=seconds,c.offset,0,true
    blizzardPhaseCursors.bySalt[salt]=c
    return c.offset,0
}

// The drawing work still scales with occupied cells, while schedule searches
// now scale only with newly crossed phases. No cursor is included in network
// packets or persisted in room history.
