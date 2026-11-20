package main

import (
    "bytes"
    "encoding/json"
    "strings"
    "testing"
    "time"
)

// Stopping locally must suppress a *room-owned* effect, not merely remove
// a self override and immediately reveal the same room animation.
func TestStopFXSelfHidesRoomEffectUntilStartFX(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    m.room="lobby"
    m.fxRoom=chatFXState{name:"rain"}
    m.terminalRoomUntil=time.Now().Add(time.Minute)
    if m.currentChatFX().name!="rain" || !m.terminalEnabled() {
        t.Fatal("room effects should initially be visible")
    }
    m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
    if m.currentChatFX().name!="" || !m.fxRoomMuted || m.fxLocalOverride || m.fxRoom.name!="rain" {
        t.Fatal("local stop failed to hide the room effect")
    }
    if m.terminalEnabled() {t.Fatal("room terminal FX remained visible after local stop")}
    m.chatFXCommand("/enablefx",[]string{"/enablefx"})
    if m.currentChatFX().name!="" {
        t.Fatal("/enablefx must not override an explicit local stop")
    }
    m.chatFXCommand("/startfx",[]string{"/startfx"})
    if m.currentChatFX().name!="rain" || !m.terminalEnabled() {
        t.Fatal("/startfx failed to restore the still-active room layers")
    }
}

// The room stop command is authoritative; /startfx can resend the last room
// effect only when it is still the same room and the client is connected.
func TestStartFXRestartsLastStoppedRoomEffect(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    m.room="lobby"
    m.connected=true
    var wire bytes.Buffer
    m.client=&wireClient{enc:json.NewEncoder(&wire)}
    m.receiveChatFX(Packet{Type:"fx_state",FX:"snow"})
    m.receiveChatFX(Packet{Type:"fx_state"})
    if m.fxLastStopped!="snow" || m.currentChatFX().name!="" {
        t.Fatal("last stopped room effect not remembered")
    }
    m.chatFXCommand("/startfx",[]string{"/startfx"})
    var packet Packet
    if err:=json.NewDecoder(&wire).Decode(&packet);err!=nil {t.Fatal(err)}
    if packet.Type!="fx_start" || packet.Room!="lobby" || packet.FX!="snow" {
        t.Fatalf("/startfx should request room effect restart: %+v",packet)
    }
    if m.currentChatFX().name!="" {
        t.Fatal("room FX must await server confirmation")
    }
}

func TestLocalStopDoesNotMuteANewRoomEffect(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    m.fxRoom=chatFXState{name:"rain"}
    m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
    if m.currentChatFX().name!="" {t.Fatal("local stop didn't hide rain")}
    m.receiveChatFX(Packet{Type:"fx_state",FX:"snow"})
    if m.currentChatFX().name!="snow" {
        t.Fatal("a newly chosen room effect should not inherit the old local stop")
    }
}

func TestRainSkyStylingDoesNotInheritMessageReverseVideo(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    m.fxRoom=chatFXState{name:"rain"}
    m.fxSimSteps=900
    // Deterministically choose a seed that actually paints the first row.
    // Otherwise the test could accidentally inspect a blank frame.
    found:=false
    for salt:=uint64(1);salt<256;salt++ {
        m.fxSalt=salt
        for x:=range m.rainPositions(100,22)[0] {
            if x>5 {found=true;break}
        }
        if found {break}
    }
    if !found {t.Fatal("no test seed painted rain across the first row")}
    rows:=make([]string,22)
    rows[0]="\x1b[7m\x1b[38;2;180;220;255mHello"+strings.Repeat(" ",95)
    for y:=1;y<len(rows);y++ {rows[y]=strings.Repeat(" ",100)}
    m.renderChatFXBackground(rows,100)
    m.renderStormSkyGradient(rows,100)
    if !strings.HasPrefix(stripANSISequences(rows[0]),"Hello") {
        t.Fatal("weather painting overwrote message text")
    }
    // A reset must separate the protected message prefix from the
    // unoccupied particle tail, preventing reverse-video/blue colour bleed.
    if !strings.Contains(rows[0],"Hello\x1b[0m") {
        t.Fatalf("rain renderer inherited unterminated reverse-video styles: %q",rows[0])
    }
}

func TestInterrogationFadeDoesNotTintSubsequentRain(t *testing.T) {
    m:=newMessageInteractionTestModel(80)
    m.fxInterrogationAmount=1
    m.fxRoom=chatFXState{name:"interrogation"}
    source:=[]string{strings.Repeat(" ",70),strings.Repeat(" ",70)}
    lit:=append([]string(nil),source...)
    m.renderInterrogationRows(lit,0)
    if lit[0]==source[0] {t.Fatal("interrogation not active in test")}
    m.fxRoom=chatFXState{name:"rain"}
    after:=append([]string(nil),source...)
    m.renderInterrogationRows(after,0)
    if after[0]!=source[0] || after[1]!=source[1] {
        t.Fatal("fading interrogation light leaked into newly launched rain")
    }
    row:=m.renderInterrogationTypingRow(source[0],0)
    if row!=source[0] {
        t.Fatal("interrogation light leaked into typing row during rain")
    }
}

func TestGlitchHasVisibleNULAndLongDiagnostics(t *testing.T) {
    haveBlock,haveNUL:=false,false
    for _,glyph:=range digitalGlitchGlyphs {
        if glyph=='█' {haveBlock=true}
        if glyph=='␀' {haveNUL=true}
        if glyph==0 {t.Fatal("raw NUL would corrupt terminal rendering")}
    }
    if !haveBlock || !haveNUL {t.Fatal("glitch palette needs solid blocks and visible NUL symbols")}
    foundTen:=false
    for i:=uint64(0);i<1024;i++ {
        seed:=chatFXHash(i)
        if 2+int((seed>>43)%9)==10 {
            foundTen=true
            if glitchErrorDuration(seed)==0 {t.Fatal("ten-line diagnostic has no duration")}
            break
        }
    }
    if !foundTen {t.Fatal("ten-line glitch diagnostics are unreachable")}
}

func TestStartFXRestoresStoppedSelfEffectWithoutNetwork(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    m.room="lobby"
    m.chatFXCommand("/bubbles",[]string{"/bubbles","self"})
    if m.currentChatFX().name!="bubbles" {t.Fatal("local bubbles did not start")}
    m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
    if m.currentChatFX().name!="" {t.Fatal("local effect still visible")}
    m.chatFXCommand("/startfx",[]string{"/startfx"})
    if m.currentChatFX().name!="bubbles" || !m.fxLocalOverride {
        t.Fatal("startfx didn't restore the last self-launched effect")
    }
}
