package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChatFXDuration(t *testing.T) {
	for _, input := range [][]string{nil, {}, {"1"}, {"30"}, {"300"}} {
		if _, ok := chatFXDuration(input); !ok { t.Fatalf("rejected %v",input) }
	}
	for _, input := range [][]string{{"0"},{"301"},{"-1"},{"cat"},{"10","20"}} {
		if _, ok := chatFXDuration(input); ok { t.Fatalf("accepted %v",input) }
	}
}

func TestChatFXLocalAndRoomState(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.user.Name = "Alice"
	m.room = "lobby"
	m.connected = true
	var outgoing bytes.Buffer
	m.client = &wireClient{enc:json.NewEncoder(&outgoing)}
	m.chatFXCommand("/rain",[]string{"/rain","self","15"})
	if !m.fxLocalOverride || m.fxLocal.name!="rain" { t.Fatal("local rain did not start") }
	if outgoing.Len()!=0 { t.Fatal("self FX must not send packets") }
	m.chatFXCommand("/rain",[]string{"/rain","8"})
	var packet Packet
	if err:=json.NewDecoder(&outgoing).Decode(&packet); err!=nil { t.Fatal(err) }
	if packet.Type!="fx_start" || packet.FX!="rain" || packet.FXSeconds!=8 {
		t.Fatalf("invalid room broadcast: %+v",packet)
	}
	m.receiveChatFX(Packet{Type:"fx_state",FX:"rain"})
	if m.currentChatFX().name!="rain" { t.Fatal("no rain active") }
	m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
	if m.fxLocalOverride || m.currentChatFX().name!="rain" {
		t.Fatal("stopping local FX should reveal room rain")
	}
	m.chatFXCommand("/disablefx",[]string{"/disablefx"})
	if m.currentChatFX().name!="" || !m.fxDisabled { t.Fatal("disablefx did not suppress rain") }
	m.chatFXCommand("/disablefx",[]string{"/disablefx"})
	if m.currentChatFX().name!="rain" { t.Fatal("reenabling FX did not restore room rain") }
	m.receiveChatFX(Packet{Type:"fx_state"})
	if m.currentChatFX().name!="" { t.Fatal("server stop did not clear rain") }
}

func TestChatFXRainNeverReplacesChatText(t *testing.T) {
	m:=newMessageInteractionTestModel(80)
	m.fxRoom=chatFXState{name:"rain"}
	m.fxSalt=17
	original:=[]string{
		"  [17:13] Alice: This text must stay readable!                             ",
		"  [17:14] Bob: And this one too.                                            ",
		strings.Repeat(" ", 78),
		strings.Repeat(" ", 78),
	}
	foundRain:=false
	for frame:=uint64(0);frame<40;frame++ {
		m.fxFrame=frame
		rows:=append([]string(nil),original...)
		m.renderChatFXBackground(rows,78)
		for i:=0;i<2;i++ {
			visible:=stripANSISequences(rows[i])
			if !strings.HasPrefix(visible,strings.TrimRight(original[i]," ")) {
				t.Fatalf("rain obscured text on row %d: %q",i,visible)
			}
		}
		if strings.Contains(stripANSISequences(strings.Join(rows,"")), "·") ||
			strings.Contains(stripANSISequences(strings.Join(rows,"")), "│") {
			foundRain=true
		}
	}
	if !foundRain { t.Fatal("rain never appeared in blank cells") }
}

func TestChatFXTimedExpiryAndOverlayVisibility(t *testing.T) {
	m:=newMessageInteractionTestModel(80)
	m.fxRoom=chatFXState{name:"rain"}
	m.fxLocal=chatFXState{name:"rain",until:time.Now().Add(-time.Second)}
	m.fxLocalOverride=true
	if m.currentChatFX().name!="rain" { t.Fatal("expired local FX hid active room FX") }
	m.fxRoom=chatFXState{}
	if m.currentChatFX().name!="" { t.Fatal("expired local FX did not stop") }
	m.fxRoom=chatFXState{name:"rain"}
	m.splashOpen=true
	if m.chatFXVisible() { t.Fatal("chat FX should not cover splash panel") }
}
