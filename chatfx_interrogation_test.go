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
        if got:=stripANSISequences(visible[i]);got!=stripANSISequences(original[i]) {
            t.Fatalf("lighting changed message characters on row %d",start+i)
        }
    }
    // A spotlight boosts the latest message, but remains subject to the
    // point-light falloff. No row is exempted from the vignette entirely.
    if visible[len(visible)-1]==original[len(original)-1] {
        t.Fatal("latest message escaped the point-light treatment")
    }
    m.messages=append(m.messages,Packet{Type:"system",Text:"Interrogation started"})
    if got:=m.interrogationSpotlightIndex();got!=newest {
        t.Fatalf("system notice stole spotlight: %d",got)
    }
}

func TestInterrogationPointLightVignetteAndFlicker(t *testing.T) {
    const width,height=100,30
    center:=interrogationLightAt(50,28,width,height,0,1)
    edge:=interrogationLightAt(0,28,width,height,0,1)
    top:=interrogationLightAt(50,0,width,height,0,1)
    corner:=interrogationLightAt(0,0,width,height,0,1)
    if !(center>edge && center>top && edge>corner) {
        t.Fatalf("no rounded bottom-centred light: centre=%f edge=%f top=%f corner=%f",center,edge,top,corner)
    }
    if corner<0.23 || center>1 {t.Fatal("lighting leaves safe intensity range")}
    emphasized:=interrogationLightAt(50,28,width,height,1,1)
    if emphasized!=center {t.Fatal("message identity changed the fixed light map")}
    if interrogationLightAt(50,28,width,height,0,0.88)>=center {
        t.Fatal("lamp flicker has no effect")
    }
    dips,steady:=0,0
    for tick:=uint64(0);tick<3900;tick++ {
        strength:=interrogationLampIntensity(tick,127)
        if strength<0.85 || strength>1 {t.Fatalf("unsafe flicker strength %f",strength)}
        if strength<1 {dips++} else {steady++}
    }
    if dips==0 || steady<dips*50 {
        t.Fatalf("flicker too frequent or absent: dips=%d steady=%d",dips,steady)
    }
}

func TestInterrogationPerCharacterLightingPreservesMessageAndBackground(t *testing.T) {
    row:="\x1b[48;2;17;19;24m\x1b[38;2;230;230;230mHello "+
        "\x1b[38;2;255;20;20mred\x1b[0m rest"
    lit:=interrogationLightRow(row,"#111318","#E6E6E6",80,25,22,1,0,1)
    // The spatial renderer intentionally pads short rows to the complete
    // chat width so its background vignette reaches the right edge.
    originalText:=stripANSISequences(row)
    litText:=stripANSISequences(lit)
    if litText!=originalText+strings.Repeat(" ",80-len(originalText)) {
        t.Fatalf("spatial lighting changed content beyond background padding: got %q, original %q",litText,originalText)
    }
    if !strings.Contains(lit,"\x1b[48;2;17;19;24m") {
        t.Fatal("spatial lighting damaged original background")
    }
    if got:=interrogationLightRow(row,"#111318","#E6E6E6",80,25,22,0,0,1);got!=row {
        t.Fatal("disabled light must preserve original ANSI bytes")
    }
    if lit==row {t.Fatal("spatial lighting not applied")}
}

func TestInterrogationLightIsStationaryWhenNewMessagesArrive(t *testing.T) {
    m:=newRenderCacheTestModel()
    m.rebuildMessageCache()
    m.fxInterrogationAmount=1
    row:="\x1b[48;2;17;19;24m\x1b[38;2;230;230;230mHello" +
        strings.Repeat(" ",chatContentWidth(m.width)-5)
    before:=[]string{row,row,row}
    m.renderInterrogationRows(before,0)
    // Newly appended messages must not restart the fade for a single row.
    m.messages=append(m.messages,Packet{Type:"message",User:User{Name:"Alice"},Text:"new testimony"})
    after:=[]string{row,row,row}
    m.renderInterrogationRows(after,10)
    for i:=range before {
        if before[i]!=after[i] {
            t.Fatalf("fixed lamp changed intensity at viewport row %d when history grew",i)
        }
    }
    // The older row above a new message has a smooth coordinate-based
    // progression rather than a separate message-specific brightness boost.
    upper:=interrogationLightAt(40,18,80,24,0,1)
    lower:=interrogationLightAt(40,19,80,24,0,1)
    if lower<upper || lower-upper>0.12 {
        t.Fatalf("abrupt vertical lighting jump: upper=%f lower=%f",upper,lower)
    }
}

func TestInterrogationTypingRowSharesMessageGradient(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    m.fxInterrogationAmount=1
    width:=chatContentWidth(m.width)
    row:="\x1b[48;2;17;19;24m\x1b[38;2;230;230;230m  Alice is typing..." +
        strings.Repeat(" ",width-len("  Alice is typing..."))
    m.fxSimSteps=100
    lastMessage:=[]string{row}
    // A one-row viewport is enough to verify continuity with the row below.
    m.renderInterrogationRows(lastMessage,0)
    typing:=m.renderInterrogationTypingRow(row,1)
    if typing==row || !strings.Contains(typing,"\x1b[48;2;") {
        t.Fatal("typing row escaped the vignette")
    }
    if stripANSISequences(typing)!=stripANSISequences(row) {
        t.Fatal("lighting altered the typing notification")
    }
    if stripANSISequences(lastMessage[0])!=stripANSISequences(row) {
        t.Fatal("lighting altered message text")
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
