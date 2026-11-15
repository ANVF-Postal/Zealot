package main

import (
    "strings"
    "testing"
    "time"
)

func TestInterrogationFadeInAndOutOverFiveSeconds(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    start:=time.Date(2026,11,15,12,0,0,0,time.UTC)
    m.fxRoom=chatFXState{name:"interrogation"}
    m.advanceInterrogationFade(start)
    if m.fxInterrogationAmount!=0 {t.Fatal("interrogation should start undimmed")}
    m.advanceInterrogationFade(start.Add(2500*time.Millisecond))
    if m.fxInterrogationAmount<0.49 || m.fxInterrogationAmount>0.51 {
        t.Fatalf("expected half dim after 2.5 seconds, got %f",m.fxInterrogationAmount)
    }
    m.advanceInterrogationFade(start.Add(5*time.Second))
    if m.fxInterrogationAmount!=1 {t.Fatalf("did not reach full dim: %f",m.fxInterrogationAmount)}
    m.fxRoom=chatFXState{}
    if m.ensureChatFXTick()==nil {t.Fatal("stopping interrogation must keep fade-out timer alive")}
    m.advanceInterrogationFade(start.Add(7500*time.Millisecond))
    if m.fxInterrogationAmount<0.49 || m.fxInterrogationAmount>0.51 {
        t.Fatalf("expected halfway restored at 2.5 seconds: %f",m.fxInterrogationAmount)
    }
    m.advanceInterrogationFade(start.Add(10*time.Second))
    if m.fxInterrogationAmount!=0 {t.Fatalf("interrogation didn't restore original brightness: %f",m.fxInterrogationAmount)}
}

func TestInterrogationPreservesANSITextAndBackground(t *testing.T) {
    row:="\x1b[48;2;17;19;24m\x1b[38;2;230;230;230mHello "+
        "\x1b[38;2;255;20;20mred\x1b[0m rest"
    original:=stripANSISequences(row)
    faded:=interrogationDimRow(row,"#111318","#E6E6E6",1)
    if got:=stripANSISequences(faded);got!=original {
        t.Fatalf("interrogation changed message text: %q",got)
    }
    if !strings.Contains(faded,"\x1b[48;2;17;19;24m") {
        t.Fatal("interrogation changed the chat background")
    }
    if !strings.Contains(faded,"\x1b[38;2;255;20;20m") {
        t.Fatal("interrogation lost original ANSI styling")
    }
    if faded==row {t.Fatal("interrogation made no visual change")}
    if got:=interrogationDimRow(row,"#111318","#E6E6E6",0);got!=row {
        t.Fatal("opacity zero must preserve original bytes")
    }
}

func TestInterrogationSpotlightTracksNewestMultilineMessage(t *testing.T) {
    m:=newRenderCacheTestModel()
    m.rebuildMessageCache()
    m.fxInterrogationAmount=1
    newest:=m.interrogationSpotlightIndex()
    if newest!=len(m.messages)-1 {t.Fatalf("latest message index=%d",newest)}
    span:=m.messageRowRanges[newest]
    if span.End<=span.Start {t.Fatal("missing latest message rows")}
    start:=max(0,span.Start-2)
    end:=span.End
    original:=append([]string(nil),m.renderedLines[start:end]...)
    visible:=append([]string(nil),original...)
    m.renderInterrogationRows(visible,start)
    for i:=range visible {
        absolute:=start+i
        if absolute>=span.Start && absolute<span.End {
            if visible[i]!=original[i] {
                t.Fatalf("latest message was dimmed at row %d",absolute)
            }
        } else if strings.TrimSpace(stripANSISequences(original[i]))!="" && visible[i]==original[i] {
            t.Fatalf("older message stayed bright at row %d",absolute)
        }
    }
    // System notices such as 'Local interrogation started' must not steal
    // the spotlight from the most recent spoken message.
    m.messages=append(m.messages,Packet{Type:"system",Text:"Interrogation started"})
    if got:=m.interrogationSpotlightIndex();got!=newest {
        t.Fatalf("system notice stole spotlight: %d",got)
    }
}

func TestInterrogationCommandUsesNormalScopes(t *testing.T) {
    m:=newMessageInteractionTestModel(80)
    if !isChatFXCommand("/interrogation") {
        t.Fatal("interrogation isn't recognized as ChatFX")
    }
    m.chatFXCommand("/interrogation",[]string{"/interrogation","self"})
    if m.currentChatFX().name!="interrogation" || !m.fxLocalOverride {
        t.Fatal("local interrogation did not start")
    }
    m.fxInterrogationAmount=0.5
    m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
    if m.currentChatFX().name!="" || m.fxInterrogationAmount!=0.5 {
        t.Fatal("stopping interrogation reset the fade instead of recovering smoothly")
    }
}

func BenchmarkInterrogationDimRows(b *testing.B) {
    m:=newRenderCacheTestModel()
    m.rebuildMessageCache()
    m.fxInterrogationAmount=0.8
    base:=append([]string(nil),m.renderedLines...)
    b.ReportAllocs()
    for i:=0;i<b.N;i++ {
        rows:=append([]string(nil),base...)
        m.renderInterrogationRows(rows,0)
    }
}
