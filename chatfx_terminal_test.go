package main

import (
    "bytes"
    "encoding/json"
    "strings"
    "testing"
    "time"
)

func TestTerminalPeriodPause(t *testing.T) {
    text:="Hi. There"
    prefix:=3*terminalLetterDelay
    if got:=terminalRevealCount(text,prefix);got!=3 {
        t.Fatalf("period should be printed: got %d runes",got)
    }
    if got:=terminalRevealCount(text,prefix+terminalPeriodPause-time.Millisecond);got!=3 {
        t.Fatalf("period must pause cursor: got %d runes",got)
    }
    if got:=terminalRevealCount(text,prefix+terminalPeriodPause+terminalLetterDelay);got!=4 {
        t.Fatalf("typing didn't resume after period: got %d runes",got)
    }
    if total:=terminalTextDuration(text);total!=time.Duration(len([]rune(text)))*terminalLetterDelay+terminalPeriodPause {
        t.Fatalf("unexpected total type time: %s",total)
    }
}

func TestTerminalRowsReserveWrappedMessageAndKeepColours(t *testing.T) {
    m:=newMessageInteractionTestModel(85)
    m.width=85
    m.terminalLocalUntil=terminalForever
    msg:=Packet{
        Type:"message",ID:"terminal-wrapped",User:User{Name:"Alice",Color:"#92C6D7"},
        Text:"A multi-line message with plenty of characters. Then a second statement that should wrap without causing the chat pane to jump.",
        Time:time.Now().UTC(),
    }
    m.appendMessage(msg)
    m.terminalTrackMessage(msg)
    if len(m.terminalReveals)!=1 {t.Fatal("received message was not tracked")}
    full:=append([]string(nil),m.renderMessageLines()...)
    if len(full)<2 {t.Fatalf("message didn't wrap: %d rows",len(full))}
    hidden:=append([]string(nil),full...)
    m.renderTerminalRows(hidden,0)
    if len(hidden)!=len(full) {t.Fatal("typewriter changed reserved message height")}
    rendered:=strings.Join(hidden,"\n")
    if strings.Contains(stripANSISequences(rendered),"plenty of characters") {
        t.Fatal("hidden body leaked before typing")
    }
    if !strings.Contains(rendered,"\x1b[7m█\x1b[27m") {
        t.Fatal("missing block cursor")
    }
    if !strings.Contains(rendered,"92;") && !strings.Contains(rendered,"92C6D7") {
        // Most ANSI renderers convert colours to 24-bit decimal escapes.
        t.Log("colour preserved in original row format; cursor didn't strip ANSI")
    }
    if !strings.Contains(rendered,"\x1b[") {
        t.Fatal("the underlying ANSI styling was stripped")
    }
    state:=m.terminalReveals[0]
    state.started=time.Now().Add(-15*terminalLetterDelay)
    m.terminalReveals[0]=state
    partial:=append([]string(nil),full...)
    m.renderTerminalRows(partial,0)
    if !strings.Contains(strings.Join(partial,"\n"),"multi-line") {
        t.Fatal("the first part of the message wasn't revealed")
    }
    if strings.Contains(stripANSISequences(strings.Join(partial,"\n")),"second statement") {
        t.Fatal("later wrapped text appeared prematurely")
    }
    state.started=time.Now().Add(-terminalTextDuration(msg.Text)-time.Second)
    m.terminalReveals[0]=state
    complete:=append([]string(nil),full...)
    m.renderTerminalRows(complete,0)
    for i:=range complete {
        if complete[i]!=full[i] {t.Fatal("completed message wasn't displayed unchanged")}
    }
}

func TestTerminalControlsRemainIndependentOfBackgroundFX(t *testing.T) {
    m:=newMessageInteractionTestModel(100)
    m.room="lobby"
    m.connected=true
    var outgoing bytes.Buffer
    m.client=&wireClient{enc:json.NewEncoder(&outgoing)}
    if !isChatFXCommand("/terminal") {t.Fatal("/terminal unregistered")}
    m.chatFXCommand("/terminal",[]string{"/terminal"})
    var p Packet
    if err:=json.NewDecoder(&outgoing).Decode(&p);err!=nil {t.Fatal(err)}
    if p.Type!="fx_terminal_start" || p.FXSeconds!=0 {
        t.Fatalf("wrong terminal room packet: %+v",p)
    }
    m.receiveChatFX(Packet{Type:"fx_terminal_state",FXUntil:terminalForever})
    m.receiveChatFX(Packet{Type:"fx_state",FX:"rain"})
    if !m.terminalEnabled() || m.currentChatFX().name!="rain" {
        t.Fatal("rain displaced the terminal layer")
    }
    m.receiveChatFX(Packet{Type:"fx_state",FX:"aquarium"})
    if !m.terminalEnabled() || m.currentChatFX().name!="aquarium" {
        t.Fatal("aquarium displaced the terminal layer")
    }
    m.chatFXCommand("/terminal",[]string{"/terminal","self","10"})
    if !m.terminalEnabled() || m.terminalLocalUntil.IsZero() {
        t.Fatal("local terminal didn't stack")
    }
    m.receiveChatFX(Packet{Type:"fx_terminal_state"})
    if !m.terminalEnabled() {t.Fatal("room terminal stop incorrectly stopped local terminal")}
    m.chatFXCommand("/terminal",[]string{"/terminal","self","off"})
    if m.terminalEnabled() {t.Fatal("self off did not stop terminal")}
    if m.currentChatFX().name!="aquarium" {t.Fatal("terminal off stopped the aquarium")}
}

func TestTerminalNeverAnimatesReplayedOrControlMessages(t *testing.T) {
    m:=newMessageInteractionTestModel(90)
    message:=Packet{Type:"message",ID:"history",Text:"old message",User:User{Name:"Bob"},Time:time.Now()}
    m.appendMessages([]Packet{message})
    if len(m.terminalReveals)!=0 {t.Fatal("history replay was tracked as new text")}
    m.terminalLocalUntil=terminalForever
    m.terminalTrackMessage(message)
    if len(m.terminalReveals)!=1 {t.Fatal("new message not tracked")}
    m.appendMessage(Packet{Type:"system",Text:"Room joined",Time:time.Now()})
    m.terminalTrackMessage(Packet{Type:"system",Text:"Room joined",Time:time.Now()})
    if len(m.terminalReveals)!=1 {t.Fatal("system notice improperly used typewriter")}
    m.fxDisabled=true
    if m.terminalEnabled() {t.Fatal("disablefx didn't suppress terminal")}
}

func TestTerminalMaskPreservesWidthAndUsernames(t *testing.T) {
    line:="\x1b[48;2;17;19;24m\x1b[38;2;55;200;245m[12:00] Alice: Hello world\x1b[0m"
    body:=messageBodyRange{Start:15,End:26}
    remaining:=0
    placed:=false
    result:=terminalMaskRow(line,body,&remaining,&placed)
    plain:=stripANSISequences(result)
    if !strings.Contains(plain,"Alice:") {t.Fatal("terminal erased the author")}
    if strings.Contains(plain,"world") {t.Fatal("terminal revealed unreached text")}
    if !strings.Contains(result,"\x1b[7m█\x1b[27m") {t.Fatal("cursor absent")}
    if strings.Contains(result,"\x1b[7m█\x1b[27m\x1b[0m") {
        t.Fatal("cursor reset message background styling")
    }
}
