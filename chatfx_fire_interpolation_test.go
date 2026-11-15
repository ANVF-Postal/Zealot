package main

import (
    "reflect"
    "strings"
    "testing"
)

func TestDoomFireIntermediateFramesKeepGentlePropagation(t *testing.T) {
    const width,height=90,28
    const salt=uint64(719)
    f:=newDoomFireState(width,height,salt)
    f.advance(4,salt)
    if len(f.previous)!=len(f.heat) {t.Fatal("missing prior heat snapshot")}
    previous:=append([]uint8(nil),f.previous...)
    target:=append([]uint8(nil),f.heat...)
    if reflect.DeepEqual(previous,target) {t.Fatal("fire did not propagate")}
    f.advance(5,salt)
    if !reflect.DeepEqual(f.heat,target) {
        t.Fatal("fire propagated between 7.5 Hz ticks")
    }
    changed:=false
    for i:=range target {
        old,new:=int(previous[i]),int(target[i])
        if old==new {continue}
        if got:=int(f.heatLevel(i,0));got!=old {
            t.Fatalf("interpolation start: got %d, want %d",got,old)
        }
        middle:=int(f.heatLevel(i,128))
        if middle<min(old,new)||middle>max(old,new) {
            t.Fatalf("interpolation overshot: %d to %d via %d",old,new,middle)
        }
        if got:=int(f.heatLevel(i,256));got!=new {
            t.Fatalf("interpolation end: got %d, want %d",got,new)
        }
        if (old-new>=2 || new-old>=2) && middle!=old && middle!=new {changed=true}
    }
    if !changed {t.Fatal("no intermediate heat levels produced")}
    f.advance(8,salt)
    if !reflect.DeepEqual(f.previous,target) {
        t.Fatal("successive propagation snapshots are not continuous")
    }
}

func TestDoomFireRenderChangesBetweenHeatTicks(t *testing.T) {
    const width,height=100,34
    m:=newMessageInteractionTestModel(width)
    m.fxRoom=chatFXState{name:"fire"}
    m.fxSalt=313
    m.fxFire=newDoomFireState(width,height,m.fxSalt)
    m.fxSimSteps=4
    m.fxFire.advance(4,m.fxSalt)
    initial:=append([]uint8(nil),m.fxFire.heat...)
    render:=func() []string {
        rows:=make([]string,height)
        for i:=range rows { rows[i]=strings.Repeat(" ",width) }
        m.renderDoomFireRows(rows,width)
        return rows
    }
    m.fxSimSteps=5
    m.fxSimAccumulator=0
    a:=render()
    m.fxSimAccumulator=chatFXSimulationStep/2
    b:=render()
    if reflect.DeepEqual(a,b) {
        t.Fatal("higher-FPS fire renders did not change between heat ticks")
    }
    if !reflect.DeepEqual(initial,m.fxFire.heat) {
        t.Fatal("rendering altered the heat simulation")
    }
}

func BenchmarkDoomFireInterpolatedRender(b *testing.B) {
    const width,height=160,45
    m:=newMessageInteractionTestModel(width)
    m.fxRoom=chatFXState{name:"fire"}
    m.fxSalt=313
    m.fxFire=newDoomFireState(width,height,m.fxSalt)
    base:=make([]string,height)
    for i:=range base {base[i]=strings.Repeat(" ",width)}
    b.ReportAllocs()
    for i:=0;i<b.N;i++ {
        m.fxSimSteps=uint64(i/2)
        if i%2==0 {m.fxSimAccumulator=0
        } else {m.fxSimAccumulator=chatFXSimulationStep/2}
        rows:=append([]string(nil),base...)
        m.renderDoomFireRows(rows,width)
    }
}
