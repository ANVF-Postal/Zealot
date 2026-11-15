package main

import (
    "reflect"
    "strings"
    "testing"
)

func TestGlitchUsesClassicPuttyANSIColors(t *testing.T) {
    expected:=[]string{
        "#BB0000","#00BB00","#BBBB00","#0000BB","#BB00BB","#00BBBB","#BBBBBB",
        "#FF5555","#55FF55","#FFFF55","#5555FF","#FF55FF","#55FFFF","#FFFFFF",
    }
    if !reflect.DeepEqual(digitalGlitchColors[:],expected) {
        t.Fatalf("non-PuTTY digital palette: %v",digitalGlitchColors)
    }
}

func TestGlitchTextTypesSequentiallyAndFades(t *testing.T) {
    const width,height=160,45
    m:=newMessageInteractionTestModel(width)
    m.fxSalt=631
    first:=uint64(0)
    var event glitchErrorEvent
    found:=false
    for tick:=uint64(0);tick<4000;tick++ {
        if e,active:=glitchTextEvent(tick,m.fxSalt);active {
            event=e
            first=tick
            found=true
            break
        }
    }
    if !found {t.Fatal("no typed error event")}
    if first!=event.start {t.Fatalf("event began late: %d versus %d",first,event.start)}
    m.fxSimSteps=event.start
    startRows,active:=m.glitchErrorPositions(width,height)
    if !active {t.Fatal("typing did not start")}
    atStart:=0
    for _,row:=range startRows {
        for _,cell:=range row {
            atStart++
            if cell.glyph!='▌' {t.Fatalf("first glyph should be cursor, got %q",cell.glyph)}
        }
    }
    if atStart==0 {t.Fatal("no initial cursor")}
    m.fxSimSteps=event.start+25
    after,_:=m.glitchErrorPositions(width,height)
    if reflect.DeepEqual(after,startRows) {t.Fatal("text did not type out")}
    codeFound:=false
    for _,row:=range after {
        var b strings.Builder
        for x:=0;x<width;x++ {
            if cell,ok:=row[x];ok {b.WriteRune(cell.glyph)} else {b.WriteByte(' ')}
        }
        if strings.Contains(b.String(),"0x") {codeFound=true}
    }
    if !codeFound {t.Fatal("missing hexadecimal diagnostic")}
    m.fxSimSteps=event.start+event.duration-1
    fadeRows,active:=m.glitchErrorPositions(width,height)
    if !active {t.Fatal("error deactivated before fade finished")}
    for _,row:=range fadeRows {
        for _,cell:=range row {
            if cell.color=="" || cell.background!="" {t.Fatal("error text lost muted foreground")}
        }
    }
    m.fxSimSteps=event.start+event.duration
    if _,active:=m.glitchErrorPositions(width,height);active {t.Fatal("error persisted after fade")}
}

func TestGlitchTextSuppressesEveryOtherGlitch(t *testing.T) {
    const width,height=160,45
    m:=newMessageInteractionTestModel(width)
    m.fxSalt=631
    var event glitchErrorEvent
    found:=false
    for tick:=uint64(0);tick<4000;tick++ {
        if e,active:=glitchTextEvent(tick,m.fxSalt);active {
            event=e
            found=true
            break
        }
    }
    if !found {t.Fatal("no text event")}
    for age:=uint64(0);age<event.duration;age+=5 {
        m.fxSimSteps=event.start+age
        expected,active:=m.glitchErrorPositions(width,height)
        if !active {t.Fatalf("error exited at age %d",age)}
        actual:=m.glitchPositions(width,height)
        if !reflect.DeepEqual(actual,expected) {
            t.Fatalf("colour noise or major distortions interrupted the text error at age %d",age)
        }
    }
    begin:=glitchDistortionTick(event.start,m.fxSalt)
    end:=glitchDistortionTick(event.start+event.duration,m.fxSalt)
    if begin!=end {t.Fatalf("digital-glitch clock advanced while text active: %d to %d",begin,end)}
}

func TestGlitchFakeErrorsFitSmallScreensAndProtectChat(t *testing.T) {
    for _,size:=range [][2]int{{1,1},{12,4},{80,24},{240,50}} {
        width,height:=size[0],size[1]
        m:=newMessageInteractionTestModel(width)
        m.fxSalt=631
        for tick:=uint64(0);tick<3600;tick+=7 {
            m.fxSimSteps=tick
            for y,row:=range m.glitchPositions(width,height) {
                for x,cell:=range row {
                    if x<0 || x>=width || y<0 || y>=height || cell.glyph==' ' || cell.color=="" {
                        t.Fatalf("invalid %dx%d error cell (%d,%d)",width,height,x,y)
                    }
                }
            }
        }
    }
    m:=newMessageInteractionTestModel(160)
    m.fxRoom=chatFXState{name:"glitch"}
    m.fxSalt=631
    prefix:="  [12:00] Alice: Actual chat must remain readable!"
    for tick:=uint64(0);tick<3000;tick+=7 {
        m.fxSimSteps=tick
        rows:=make([]string,35)
        for y:=range rows {rows[y]=strings.Repeat(" ",160)}
        rows[5]=prefix+strings.Repeat(" ",160-len(prefix))
        m.renderChatFXBackground(rows,160)
        if !strings.HasPrefix(stripANSISequences(rows[5]),prefix) {
            t.Fatal("error text overwrote chat")
        }
    }
}

func BenchmarkGlitchTextAndColorBackground(b *testing.B) {
    m:=newMessageInteractionTestModel(160)
    m.fxSalt=631
    b.ReportAllocs()
    for i:=0;i<b.N;i++ {
        m.fxSimSteps=uint64(i)
        m.glitchPositions(160,45)
    }
}
