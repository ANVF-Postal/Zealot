package main

import (
	"fmt"
	"math"
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


func TestChatFXSimulationIndependentOfRenderFPS(t *testing.T) {
	start := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, fps := range []int{1, 5, 15, 30, 60} {
		m := newMessageInteractionTestModel(80)
		m.fxRoom = chatFXState{name: "rain"}
		m.fxRenderFPS = fps
		m.advanceChatFXClock(start)
		step := m.chatFXFrameInterval()
		if step != time.Second/time.Duration(fps) {
			t.Fatalf("%d FPS interval is %v", fps, step)
		}
		// Simulate roughly a second's worth of frame callbacks.
		for i := 1; i <= fps; i++ {
			m.advanceChatFXClock(start.Add(time.Duration(i)*time.Second/time.Duration(fps)))
		}
		if m.fxSimSteps < 29 || m.fxSimSteps > 31 {
			t.Fatalf("FPS %d: simulation advanced %d steps, expected ~30",fps,m.fxSimSteps)
		}
	}
}

func TestChatFXInitialViewportIsFullyPopulated(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.fxRoom = chatFXState{name:"rain"}
	m.fxSalt = 73
	// No resize or tick has happened. Particles must cover the initial
	// viewport's whole height and width.
	for _, size := range [][2]int{{100,28},{40,12},{150,40}} {
		width,height:=size[0],size[1]
		positions := m.rainPositions(width,height)
		if len(positions)!=height { t.Fatalf("positions have %d rows, want %d",len(positions),height) }
		left,right,top,bottom:=false,false,false,false
		for y,row := range positions {
			for x := range row {
				if x < width/3 { left = true }
				if x >= width*2/3 { right = true }
				if y < height/3 { top = true }
				if y >= height*2/3 { bottom = true }
			}
		}
		if !left || !right || !top || !bottom {
			t.Fatalf("initial viewport %dx%d has incomplete coverage: L%v R%v T%v B%v",
				width,height,left,right,top,bottom)
		}
	}
}

func TestChatFXFPSCommandIsLocalAndValidated(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.chatFXCommand("/fxfps",[]string{"/fxfps","60"})
	if m.fxRenderFPS!=60 { t.Fatalf("fps = %d",m.fxRenderFPS) }
	for _, input := range []string{"0","61","wrong"} {
		m.chatFXCommand("/fxfps",[]string{"/fxfps",input})
		if m.fxRenderFPS!=60 { t.Fatalf("invalid FPS %q changed setting",input) }
	}
	m.chatFXCommand("/fxfps",[]string{"/fxfps","1"})
	if m.fxRenderFPS!=1 { t.Fatal("1 FPS should be allowed") }
}


func TestChatFXFPSIsHandledThroughActualChatCommand(t *testing.T) {
	m := newMessageInteractionTestModel(90)
	for _, fps := range []int{60, 20, 1, 45} {
		text := fmt.Sprintf("/fxfps %d", fps)
		if sent := m.sendMessage(text, ""); sent {
			t.Fatalf("%q was sent as a public chat message", text)
		}
		if m.fxRenderFPS != fps {
			t.Fatalf("%q left render FPS at %d",text,m.fxRenderFPS)
		}
		if m.chatFXFrameInterval()!=time.Second/time.Duration(fps) {
			t.Fatalf("timer did not update to %d FPS", fps)
		}
	}
}

func TestConfettiCommandRoomAndSelf(t *testing.T) {
	m := newMessageInteractionTestModel(90)
	m.room = "lobby"
	m.connected = true
	var outgoing bytes.Buffer
	m.client = &wireClient{enc:json.NewEncoder(&outgoing)}
	if m.sendMessage("/confetti self", "") { t.Fatal("self confetti sent as chat text") }
	if outgoing.Len()!=0 { t.Fatal("local confetti sent a packet") }
	if m.currentChatFX().name!="confetti" || m.fxLocal.until.IsZero() {
		t.Fatal("local confetti must be active and timed")
	}
	m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
	if m.sendMessage("/confetti 12","") { t.Fatal("room confetti sent as chat text") }
	var packet Packet
	if err:=json.NewDecoder(&outgoing).Decode(&packet);err!=nil { t.Fatal(err) }
	if packet.Type!="fx_start" || packet.FX!="confetti" || packet.FXSeconds!=12 {
		t.Fatalf("wrong confetti request: %+v",packet)
	}
	m.receiveChatFX(Packet{Type:"fx_state", FX:"confetti", FXUntil:time.Now().Add(time.Second)})
	if m.currentChatFX().name!="confetti" { t.Fatal("room confetti not visible") }
	m.receiveChatFX(Packet{Type:"fx_state", FX:"rain"})
	if m.currentChatFX().name!="rain" { t.Fatal("primary effects stacked instead of replacing") }
}

func TestRainDensityHasVariableSmoothPhases(t *testing.T) {
	for _,salt := range []uint64{1,19,999} {
		for i:=0;i<3600;i++ {
			d := rainDensityAt(float64(i),salt)
			if d<0.28-1e-8 || d>0.98+1e-8 { t.Fatalf("out-of-range density %f",d) }
		}
		initial:=rainDensityAt(0,salt)
		for i:=1;i<=30;i++ {
			if rainDensityAt(float64(i),salt)!=initial { t.Fatalf("initial state should linger at least 30s for salt %d",salt) }
		}
		minDensity,maxDensity:=1.0,0.0
		for i:=0;i<1800;i++ {
			d:=rainDensityAt(float64(i),salt)
			minDensity=min(minDensity,d)
			maxDensity=max(maxDensity,d)
			if i>0 && math.Abs(d-rainDensityAt(float64(i-1),salt))>0.5 {
				t.Fatalf("abrupt jump in rain density at %ds",i)
			}
		}
		if maxDensity-minDensity<0.05 { t.Fatalf("rain density never varied for salt %d",salt) }
	}
}

func TestConfettiParticlesCandyGlyphsAndRareStreamers(t *testing.T) {
	m:=newMessageInteractionTestModel(95)
	m.fxRoom=chatFXState{name:"confetti"}
	m.fxSalt=27
	seen:=map[rune]bool{}
	colors:=map[string]bool{}
	foundStreamer:=false
	for i:=0;i<180;i++ {
		m.fxSimSteps=uint64(i*5)
		positions:=m.confettiPositions(95,26)
		for y,row:=range positions {
			for x,cell:=range row {
				if x<0 || x>=95 || y<0 || y>=26 { t.Fatal("particle left viewport") }
				seen[cell.glyph]=true
				colors[cell.color]=true
				if (cell.glyph=='/' || cell.glyph=='\\') && y>0 {
					if above,ok:=positions[y-1][x];ok && (above.glyph=='/' || above.glyph=='\\') {
						foundStreamer=true
					}
				}
			}
		}
	}
	if len(seen)<6 { t.Fatalf("too few confetti glyphs: %v",seen) }
	if len(colors)<6 { t.Fatalf("too few candy color variants: %v",colors) }
	if !foundStreamer { t.Fatal("no streamers found in confetti") }
}

func TestConfettiNeverObscuresChatText(t *testing.T) {
	m:=newMessageInteractionTestModel(95)
	m.fxRoom=chatFXState{name:"confetti"}
	m.fxSalt=29
	m.fxSimSteps=120
	original:=[]string{
		"  [12:00] Someone: The cake is a lie.           ",
		strings.Repeat(" ", 60),
		strings.Repeat(" ", 60),
		strings.Repeat(" ", 60),
	}
	rows:=append([]string(nil),original...)
	m.renderChatFXBackground(rows,60)
	if !strings.HasPrefix(stripANSISequences(rows[0]),strings.TrimRight(original[0]," ")) {
		t.Fatalf("confetti obscured message: %s",stripANSISequences(rows[0]))
	}
}
