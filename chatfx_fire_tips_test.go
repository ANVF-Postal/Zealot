package main

import (
    "reflect"
    "strings"
    "testing"
)

func TestDoomFireTipsSparseAboveCompactBody(t *testing.T) {
    const width,height=240,45
    state:=newDoomFireState(width,height,313)
    if state==nil {t.Fatal("missing fire state")}
    if len(state.heat)!=width*doomFireHeight(width,height) {
        t.Fatal("tips must not expand the heat grid")
    }
    foundUpper:=false
    seen:=make(map[int]bool)
    for tick:=uint64(0);tick<1200;tick+=4 {
        state.advance(tick,313)
        tips:=doomFireTips(state,height,313)
        if len(tips)>48 {t.Fatalf("too many wisps: %d",len(tips))}
        for _,tip:=range tips {
            if tip.x<0||tip.x>=width||tip.y<0||tip.y>=height-state.height {
                t.Fatalf("tip outside upper viewport: %+v",tip)
            }
            seen[tip.y]=true
            foundUpper=true
        }
        if !reflect.DeepEqual(tips,doomFireTips(state,height,313)) {
            t.Fatal("tips are not deterministic for one simulation tick")
        }
    }
    if !foundUpper || len(seen)<4 {
        t.Fatalf("tips did not rise naturally: rows=%d",len(seen))
    }
}

func TestDoomFireTipsRespectShortViewports(t *testing.T) {
    for _,size:=range [][2]int{{1,1},{12,4},{80,10},{320,45}} {
        width,height:=size[0],size[1]
        state:=newDoomFireState(width,height,991)
        for _,tip:=range doomFireTips(state,height,991) {
            if tip.x<0||tip.x>=width||tip.y<0||tip.y>=height-state.height {
                t.Fatalf("invalid %dx%d fire tip: %+v",width,height,tip)
            }
        }
    }
}

func TestDoomFireUpperTipsDoNotOverwriteMessages(t *testing.T) {
    const width,height=240,45
    m:=newMessageInteractionTestModel(width)
    m.fxRoom=chatFXState{name:"fire"}
    m.fxSalt=313
    m.fxFire=newDoomFireState(width,height,m.fxSalt)
    m.fxSimSteps=240
    rows:=make([]string,height)
    for y:=range rows {rows[y]=strings.Repeat(" ",width)}
    rows[6]="  Upper message must remain visible"
    rows[height-2]="  Lower message must remain visible"
    m.renderDoomFireRows(rows,width)
    for _,y:=range []int{6,height-2} {
        expected:="  Upper message must remain visible"
        if y==height-2 {expected="  Lower message must remain visible"}
        if !strings.HasPrefix(stripANSISequences(rows[y]),expected) {
            t.Fatalf("fire damaged chat row %d",y)
        }
    }
}

func BenchmarkDoomFireSparseUpperTips(b *testing.B) {
    for _,width:=range []int{80,160,320} {
        b.Run(string(rune('A'+width/80)),func(b *testing.B) {
            state:=newDoomFireState(width,45,313)
            b.ReportAllocs()
            for i:=0;i<b.N;i++ {
                state.advance(uint64(i),313)
                doomFireTips(state,45,313)
            }
        })
    }
}
