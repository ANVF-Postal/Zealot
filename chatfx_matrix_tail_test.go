package main

import (
    "math"
    "testing"
    "time"
)

// A long Matrix stream must finish crossing the bottom before its modulo
// clock wraps. Regression for trails vanishing when the head has gone
// only seven rows below the viewport.
func TestMatrixLongTailLeavesViewportBeforeWrap(t *testing.T) {
    const width,height=160,45
    m:=newMessageInteractionTestModel(width)
    m.fxSalt=71
    column:=-1
    length:=0
    var speed,phase float64
    for x:=0;x<width;x++ {
        seed:=chatFXHash(uint64(x+1)*0x9e3779b97f4a7c15+m.fxSalt)
        if chatFXUnit(seed+1)>0.61 {continue}
        l:=4+int(chatFXHash(seed+2)%uint64(max(3,min(16,height+3))))
        if l<14 {continue}
        column=x
        length=l
        speed=3.6+8.2*chatFXUnit(seed+3)
        phase=chatFXUnit(seed+4)*float64(height+length)
        break
    }
    if column<0 {t.Fatal("no sufficiently long test stream")}
    // Sample the next complete cycle so elapsed is always nonnegative,
    // irrespective of the stream's initial random phase.
    period:=float64(height+2*length+7)
    sample:=func(head int) []map[int]chatFXCell {
        elapsed:=(period+float64(head+length)+0.25-phase)/speed
        steps:=elapsed*30
        whole:=math.Floor(steps)
        m.fxSimSteps=uint64(whole)
        m.fxSimAccumulator=chatFXSimulationStep * time.Duration((steps-whole)*1000000) / 1000000
        return m.matrixPositions(width,height)
    }
    // At every head row from the bottom border to the final trailing
    // character, one or more characters should remain visible.
    for head:=height-1;head<height+length-1;head++ {
        rows:=sample(head)
        n:=0
        for _,row:=range rows {
            if _,present:=row[column];present {n++}
        }
        expected:=min(length,height+length-1-head)
        if n!=expected {
            t.Fatalf("head %d: %d visible tail cells; expected %d",head,n,expected)
        }
    }
    // Once the last character exits, it must remain absent until its
    // next natural cycle. In particular there must be no premature reset.
    for head:=height+length-1;head<=height+length+5;head++ {
        rows:=sample(head)
        for _,row:=range rows {
            if _,present:=row[column];present {
                t.Fatalf("stream reappeared at off-screen head %d",head)
            }
        }
    }
}
