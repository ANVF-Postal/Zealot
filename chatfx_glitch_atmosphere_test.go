package main

import (
    "reflect"
    "testing"
)

// The minor interference clock must not repeat a fixed pulse period, and
// moving backward on the simulation timeline must reproduce the same frame.
func TestMinorGlitchIrregularPulseSchedule(t *testing.T) {
    const salt uint64=91
    starts:=[]uint64{}
    lengths:=[]uint64{}
    active:=false
    var start uint64
    for tick:=uint64(0);tick<18000;tick++ {
        pulse:=minorGlitchPulse(tick,salt)
        if pulse<0 || pulse>1 {
            t.Fatalf("minor Glitch envelope out of bounds at %d: %f",tick,pulse)
        }
        on:=pulse>0
        if on && !active {start=tick;starts=append(starts,start)}
        if !on && active {lengths=append(lengths,tick-start)}
        active=on
    }
    if len(starts)<35 || len(lengths)<35 {t.Fatalf("minor Glitch too sparse: %d events",len(starts))}
    gaps:=map[uint64]bool{}
    durations:=map[uint64]bool{}
    for i:=1;i<len(starts);i++ {gaps[starts[i]-starts[i-1]]=true}
    for _,n:=range lengths {durations[n]=true}
    if len(gaps)<15 || len(durations)<15 {
        t.Fatalf("minor Glitch repeats too predictably: %d distinct gaps, %d durations",len(gaps),len(durations))
    }
    saved:=make(map[uint64]float64)
    for _,tick:=range []uint64{0,31,510,1000,4800,9000,17500} {
        saved[tick]=minorGlitchPulse(tick,salt)
    }
    for _,tick:=range []uint64{17500,9000,31,4800,0,1000,510} {
        if got:=minorGlitchPulse(tick,salt);got!=saved[tick] {
            t.Fatalf("minor Glitch changed after seeking backward to %d: got %f expected %f",tick,got,saved[tick])
        }
    }
}

func TestGlitchDiagnosticMemoryBleedIsSubtleAndTemporary(t *testing.T) {
    const width,height=160,45
    m:=newMessageInteractionTestModel(width)
    m.fxSalt=631
    var event glitchErrorEvent
    found:=false
    for tick:=uint64(0);tick<4000;tick++ {
        if candidate,ok:=glitchTextEvent(tick,m.fxSalt);ok {
            event=candidate
            found=true
            break
        }
    }
    if !found {t.Fatal("no diagnostic event")}
    m.fxSimSteps=event.start
    initial,ok:=m.glitchErrorPositions(width,height)
    if !ok {t.Fatal("diagnostic did not start")}
    for _,row:=range initial {
        for _,cell:=range row {
            if cell.glyph!='▌' {t.Fatalf("memory bleed obscured the initial cursor: %q",cell.glyph)}
        }
    }
    // Writing has been underway long enough for the halo to appear.
    m.fxSimSteps=event.start+90
    withHalo,ok:=m.glitchErrorPositions(width,height)
    if !ok {t.Fatal("diagnostic ended too early")}
    foundHalo:=false
    for _,row:=range withHalo {
        for _,cell:=range row {
            if cell.background!="" {t.Fatal("memory bleed introduced a coloured background")}
            if cell.glyph=='░' || cell.glyph=='▒' || cell.glyph=='␀' || cell.glyph=='╳' || cell.glyph=='·' {
                foundHalo=true
            }
        }
    }
    if !foundHalo {t.Fatal("memory bleed did not produce any atmospheric glyphs")}
    if !reflect.DeepEqual(withHalo,m.glitchPositions(width,height)) {
        t.Fatal("major/minor glitches interrupted the typed diagnostic")
    }
    m.fxSimSteps=event.start+event.duration-1
    closing,ok:=m.glitchErrorPositions(width,height)
    if !ok {t.Fatal("diagnostic exited before its fade")}
    for _,row:=range closing {
        for _,cell:=range row {
            if cell.glyph=='░' || cell.glyph=='▒' || cell.glyph=='␀' || cell.glyph=='╳' || cell.glyph=='·' {
                t.Fatal("memory bleed continued after typing ended")
            }
        }
    }
}
