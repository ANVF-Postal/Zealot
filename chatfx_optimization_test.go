package main

import (
    "math"
    "reflect"
    "strings"
    "testing"

    "charm.land/lipgloss/v2"
)

// Baseline schedule implementations are retained here so the optimized
// progressive cursors must match the original deterministic timelines,
// including backwards seeks and effect salts used by different clients.
func referenceRainDensity(seconds float64,salt uint64) float64 {
    if seconds<0 {seconds=0}
    previous:=0.65
    for segment:=uint64(0);segment<65536;segment++ {
        seed:=chatFXHash(salt+segment*0x9e3779b97f4a7c15)
        target:=0.28+0.70*chatFXUnit(seed+1)
        hold:=30+150*chatFXUnit(seed+2)
        transition:=3+9*chatFXUnit(seed+3)
        if seconds<hold {return previous}
        seconds-=hold
        if seconds<transition {
            u:=seconds/transition
            u=u*u*(3-2*u)
            return previous+(target-previous)*u
        }
        seconds-=transition
        previous=target
    }
    return previous
}

func referenceBlizzardWind(seconds float64,salt uint64) (offset,speed float64) {
    if seconds<=0 {return 0,0}
    remaining:=seconds
    for phase:=uint64(0);phase<65536;phase++ {
        p:=blizzardPhase(phase,salt)
        if remaining>=p.gustDuration {
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
        if remaining<p.calmDuration {return offset,0}
        remaining-=p.calmDuration
    }
    return offset,0
}

func referenceInterferenceBurst(tick,salt uint64,glitch bool) (uint64,bool) {
    next:=uint64(0)
    for event:=uint64(0);event<100000;event++ {
        seed:=chatFXHash(salt+(event+1)*0x9e3779b97f4a7c15)
        next+=interferenceGap(seed,glitch)
        length:=uint64(3+(seed>>24)%10)
        if glitch {length=uint64(4+(seed>>24)%13)}
        if tick<next {return seed,false}
        if tick<next+length {return seed,true}
        next+=length
    }
    return 0,false
}

func referenceGlitchSchedule(tick,salt uint64) (glitchErrorEvent,bool,uint64) {
    next,paused:=uint64(0),uint64(0)
    var event glitchErrorEvent
    active:=false
    for i:=uint64(0);i<100000;i++ {
        seed:=chatFXHash(salt^0x7e827aa2+(i+1)*0x9e3779b97f4a7c15)
        next+=270+(seed>>8)%720
        if tick<next {break}
        duration:=glitchErrorDuration(seed)
        if tick<next+duration {
            event=glitchErrorEvent{start:next,duration:duration,seed:seed}
            active=true
            paused+=tick-next
            break
        }
        next+=duration
        paused+=duration
    }
    return event,active,tick-paused
}

func TestChatFXCachedSchedulesMatchOriginalTimeline(t *testing.T) {
    moments:=[]float64{0,0.1,1,29,30,31,75,150,600,3600,7200,900,1,7200,0,3600}
    ticks:=[]uint64{0,1,15,100,270,500,900,1800,30000,108000,216000,2700,0,216000,108000}
    for _,salt:=range []uint64{0,73,127,99291} {
        for _,second:=range moments {
            got,want:=rainDensityAt(second,salt),referenceRainDensity(second,salt)
            if math.Abs(got-want)>1e-10 {
                t.Fatalf("rain at %.2f salt %d: got %.12f want %.12f",second,salt,got,want)
            }
            gotOffset,gotSpeed:=blizzardWindSample(second,salt)
            wantOffset,wantSpeed:=referenceBlizzardWind(second,salt)
            if math.Abs(gotOffset-wantOffset)>1e-7 || math.Abs(gotSpeed-wantSpeed)>1e-9 {
                t.Fatalf("wind at %.2f salt %d: got (%v,%v), want (%v,%v)",second,salt,gotOffset,gotSpeed,wantOffset,wantSpeed)
            }
        }
        for _,tick:=range ticks {
            for _,glitch:=range []bool{false,true} {
                gotSeed,gotActive:=interferenceBurst(tick,salt,glitch)
                wantSeed,wantActive:=referenceInterferenceBurst(tick,salt,glitch)
                if gotSeed!=wantSeed || gotActive!=wantActive {
                    t.Fatalf("interference tick %d salt %d glitch %v: got (%d,%v) want (%d,%v)",
                        tick,salt,glitch,gotSeed,gotActive,wantSeed,wantActive)
                }
            }
            gotEvent,gotActive:=glitchTextEvent(tick,salt)
            gotTick:=glitchDistortionTick(tick,salt)
            wantEvent,wantActive,wantTick:=referenceGlitchSchedule(tick,salt)
            if !reflect.DeepEqual(gotEvent,wantEvent) || gotActive!=wantActive || gotTick!=wantTick {
                t.Fatalf("glitch schedule tick %d salt %d: got (%+v,%v,%d) want (%+v,%v,%d)",
                    tick,salt,gotEvent,gotActive,gotTick,wantEvent,wantActive,wantTick)
            }
        }
    }
}

// The cursor must show the completed gust displacement throughout its calm
// interval without adding that same gust again on subsequent frames.
func TestCachedBlizzardWindStaysContinuousThroughCalm(t *testing.T) {
    for _,salt:=range []uint64{0,73,127} {
        p:=blizzardPhase(0,salt)
        times:=[]float64{
            p.gustDuration-0.1,
            p.gustDuration,
            p.gustDuration+0.01,
            p.gustDuration+p.calmDuration/2,
            p.gustDuration+p.calmDuration-0.01,
            p.gustDuration+p.calmDuration,
            p.gustDuration+p.calmDuration+0.01,
            p.gustDuration+p.calmDuration/2, // backward seek
        }
        for _,second:=range times {
            got,speed:=blizzardWindSample(second,salt)
            want,wantSpeed:=referenceBlizzardWind(second,salt)
            if math.Abs(got-want)>1e-7 || math.Abs(speed-wantSpeed)>1e-9 {
                t.Fatalf("salt %d at %.6fs: wind (%v,%v), reference (%v,%v)",salt,second,got,speed,want,wantSpeed)
            }
        }
    }
}

func TestChatFXOptimizedANSIRendererPreservesTextAndWidth(t *testing.T) {
    for _,effect:=range []string{"static","glitch","rain","snow","blizzard","matrix"} {
        m:=newMessageInteractionTestModel(100)
        m.fxRoom=chatFXState{name:effect}
        m.fxSalt=73
        m.fxSimSteps=900
        width:=chatContentWidth(m.width)
        message:="  [12:34] Alice: Please keep my message visible."
        rows:=[]string{message+strings.Repeat(" ",max(0,width-len(message))),
            strings.Repeat(" ",width),strings.Repeat(" ",width)}
        m.renderChatFXBackground(rows,width)
        m.renderStormSkyGradient(rows,width)
        for i,row:=range rows {
            if i==0 && !strings.HasPrefix(stripANSISequences(row),message) {
                t.Fatalf("%s covered message: %q",effect,stripANSISequences(row))
            }
            if got:=lipgloss.Width(row);got!=width {
                t.Fatalf("%s row %d width %d (want %d)",effect,i,got,width)
            }
        }
    }
}
