package main

import (
	"reflect"
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


func TestConfettiParticlesStayWithinViewportAtAllAges(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.fxRoom = chatFXState{name: "confetti"}
	m.fxSalt = 47
	for _, size := range [][2]int{{1, 1}, {12, 4}, {40, 12}, {95, 26}} {
		width, height := size[0], size[1]
		for second := 0; second < 60; second++ {
			m.fxSimSteps = uint64(second * 30)
			positions := m.confettiPositions(width, height)
			if len(positions) != height {
				t.Fatalf("%dx%d at %ds: got %d rows", width, height, second, len(positions))
			}
			for y, row := range positions {
				for x := range row {
					if y < 0 || y >= height || x < 0 || x >= width {
						t.Fatalf("particle outside %dx%d viewport at %ds: (%d,%d)", width, height, second, x, y)
					}
				}
			}
		}
	}
}


func TestFXFPSChangesImmediatelyDuringRainAndWhisperReply(t *testing.T) {
	m := newMessageInteractionTestModel(90)
	m.fxRoom = chatFXState{name: "rain"}
	m.fxTickPending = true // simulate a frame already scheduled at the old FPS
	m.fxTickGeneration = 17
	m.messageByID = map[string]Packet{"reply": {Type: "whisper_result", User: User{Name: "Bob"}, WhisperTo: "Alice"}}
	m.user.Name = "Alice"
	var outgoing bytes.Buffer
	m.client = &wireClient{enc: json.NewEncoder(&outgoing)}
	for _, fps := range []int{60, 20, 1, 30} {
		before := m.fxTickGeneration
		if m.sendMessage(fmt.Sprintf("/fxfps %d",fps),"reply") {
			t.Fatal("FX command should never be sent as a chat or whisper message")
		}
		if outgoing.Len() != 0 { t.Fatal("/fxfps leaked onto the wire") }
		if m.fxRenderFPS != fps { t.Fatalf("FPS stayed at %d, want %d",m.fxRenderFPS,fps) }
		if m.fxTickPending || m.fxTickGeneration != before+1 {
			t.Fatalf("timer not invalidated at %d FPS",fps)
		}
		if m.ensureChatFXTick()==nil { t.Fatalf("timer not restarted at %d FPS",fps) }
	}
}

func TestConfettiDrainsAfterEmissionEnds(t *testing.T) {
	m := newMessageInteractionTestModel(95)
	now := time.Now()
	m.fxRoom = chatFXState{
		name:"confetti", started:now.Add(-11*time.Second),
		until:now.Add(-time.Second),
	}
	m.fxSalt=27
	m.fxSimSteps=uint64(11*30)
	if m.currentChatFX().name!="confetti" {
		t.Fatal("confetti should remain active while particles are falling out")
	}
	alive := 0
	for _,row := range m.confettiPositions(95,26) { alive+=len(row) }
	if alive==0 { t.Fatal("confetti vanished immediately at emission deadline") }
	m.fxSimSteps=uint64(60*30)
	remaining := 0
	for _,row := range m.confettiPositions(95,26) { remaining+=len(row) }
	if remaining!=0 { t.Fatalf("confetti kept respawning after emission ended: %d cells",remaining) }
	m.fxRoom.until=now.Add(-2*time.Minute)
	if m.currentChatFX().name!="" { t.Fatal("confetti lingered indefinitely") }
}

func TestSnowIsGentleBoundedAndChangesIntensity(t *testing.T) {
	m := newMessageInteractionTestModel(95)
	m.fxRoom = chatFXState{name:"snow"}
	m.fxSalt=101
	for _,viewport := range [][2]int{{12,4},{95,26},{180,50}} {
		width,height:=viewport[0],viewport[1]
		for _,seconds:=range []uint64{0,1,30,90,180,300} {
			m.fxSimSteps=seconds*30
			rows:=m.snowPositions(width,height)
			if len(rows)!=height { t.Fatal("snow height mismatch") }
			for y,row:=range rows {
				for x,particle:=range row {
					if x<0||x>=width||y<0||y>=height { t.Fatal("snow fell outside viewport") }
					if particle.color=="" { t.Fatal("uncolored snowflake") }
				}
			}
		}
	}
	m.fxSimSteps=0
	initial:=m.snowPositions(95,26)
	m.fxSimSteps=5*30
	later:=m.snowPositions(95,26)
	if reflect.DeepEqual(initial,later) { t.Fatal("snow is not moving") }
	minDensity,maxDensity:=1.0,0.0
	for second:=0;second<1800;second+=30 {
		d:=rainDensityAt(float64(second),m.fxSalt^0x5eafc0de)
		minDensity=min(minDensity,d)
		maxDensity=max(maxDensity,d)
	}
	if maxDensity-minDensity<0.05 { t.Fatal("snow intensity did not vary") }
}


func TestSnowCommandRoomAndSelf(t *testing.T) {
	m:=newMessageInteractionTestModel(80)
	m.room="lobby"
	m.connected=true
	var outgoing bytes.Buffer
	m.client=&wireClient{enc:json.NewEncoder(&outgoing)}
	if m.sendMessage("/SnOw self 20","") { t.Fatal("local snow became chat text") }
	if m.currentChatFX().name!="snow" || outgoing.Len()!=0 {
		t.Fatal("self snow must only affect this client")
	}
	m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
	if m.sendMessage("/snow 15","") { t.Fatal("room snow became chat text") }
	var packet Packet
	if err:=json.NewDecoder(&outgoing).Decode(&packet);err!=nil { t.Fatal(err) }
	if packet.Type!="fx_start" || packet.FX!="snow" || packet.FXSeconds!=15 {
		t.Fatalf("invalid room snow request: %+v",packet)
	}
}

func TestSnowLeavesMessageGlyphsUntouched(t *testing.T) {
	m:=newMessageInteractionTestModel(90)
	m.fxRoom=chatFXState{name:"snow"}
	m.fxSalt=19
	m.fxSimSteps=300
	original:=[]string{
		"  [17:10] Alice: Quiet snow tonight.           ",
		strings.Repeat(" ",60),
		strings.Repeat(" ",60),
		strings.Repeat(" ",60),
	}
	rows:=append([]string(nil),original...)
	m.renderChatFXBackground(rows,60)
	if !strings.HasPrefix(stripANSISequences(rows[0]),strings.TrimRight(original[0]," ")) {
		t.Fatal("snow obscured chat text")
	}
}


func TestFireworksCommandIsRoomOrSelfScoped(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.room="lobby"
	m.connected=true
	var wire bytes.Buffer
	m.client=&wireClient{enc:json.NewEncoder(&wire)}
	if m.sendMessage("/fireworks self","") { t.Fatal("local fireworks must not send chat text") }
	if wire.Len()!=0 || m.fxLocal.name!="fireworks" || m.fxLocal.until.IsZero() {
		t.Fatal("self fireworks must be local and default to a duration")
	}
	m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
	if m.sendMessage("/fireworks 15","") { t.Fatal("room fireworks became a chat message") }
	var packet Packet
	if err:=json.NewDecoder(&wire).Decode(&packet);err!=nil { t.Fatal(err) }
	if packet.Type!="fx_start" || packet.FX!="fireworks" || packet.FXSeconds!=15 {
		t.Fatalf("unexpected fireworks request: %+v",packet)
	}
}

func TestFireworksHaveShellsTrailsAndBursts(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.fxRoom=chatFXState{name:"fireworks",started:time.Now(),until:time.Now().Add(10*time.Second)}
	m.fxSalt=229
	shells:=0
	warmTrails:=0
	bursts:=0
	colors:=map[string]bool{}
	for frame:=0;frame<120;frame++ {
		m.fxSimSteps=uint64(frame*5)
		rows:=m.fireworkPositions(100,32)
		for y,row:=range rows {
			for x,c:=range row {
				if x<0||x>=100||y<0||y>=32 { t.Fatal("fireworks escaped viewport") }
				colors[c.color]=true
				if c.glyph=='o' {shells++;if c.color!="#ECF1F5" { t.Fatal("shell must remain neutral") }}
				if c.color=="#FFBB45"||c.color=="#FF8617"||c.color=="#FFF1B4"||c.color=="#AC4E09" {
					warmTrails++
				}
				if c.glyph=='#'||c.glyph=='*'||c.glyph=='%'||c.glyph=='x' {bursts++}
			}
		}
	}
	if shells==0 || warmTrails==0 || bursts==0 || len(colors)<7 {
		t.Fatalf("missing variety: shells=%d trails=%d bursts=%d colors=%d",shells,warmTrails,bursts,len(colors))
	}
}

func TestFireworkEmissionEndsWithoutKillingBursts(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	now:=time.Now()
	m.fxRoom=chatFXState{name:"fireworks",started:now.Add(-11*time.Second),until:now.Add(-time.Second)}
	m.fxSalt=229
	m.fxSimSteps=uint64(11*30)
	if m.currentChatFX().name!="fireworks" {t.Fatal("existing fireworks should finish after cutoff")}
	m.fxSimSteps=uint64(35*30)
	for _,row:=range m.fireworkPositions(100,32) {
		if len(row)!=0 {t.Fatal("shells or sparks kept respawning after cutoff")}
	}
	m.fxRoom.until=now.Add(-time.Minute)
	if m.currentChatFX().name!="" {t.Fatal("fireworks should eventually finish")}
}

func TestReplacingRoomFXRetiresOldParticles(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.fxRoom=chatFXState{name:"snow"}
	m.fxSalt=7
	m.width=100
	m.height=32
	m.receiveChatFX(Packet{Type:"fx_state",FX:"fireworks",FXUntil:time.Now().Add(10*time.Second)})
	if m.currentChatFX().name!="fireworks" {t.Fatal("new room FX was not applied")}
	if len(m.fxFading)==0 || len(m.fxFading[0].particles)==0 {t.Fatal("old snow did not enter fade-out")}
	m.receiveChatFX(Packet{Type:"fx_state"})
	if len(m.fxFading)<2 {t.Fatal("stopping fireworks did not preserve outgoing particles")}
	if m.currentChatFX().name!="" {t.Fatal("stop must end the primary emitter")}
	if m.ensureChatFXTick()==nil {t.Fatal("fade-out must continue animating after room stop")}
}

func TestSmallSnowDotsFallFaster(t *testing.T) {
	// The dot variant is explicitly faster than the corresponding seed's
	// regular snowflake speed; verify the range and that both appear.
	m:=newMessageInteractionTestModel(90)
	m.fxRoom=chatFXState{name:"snow"}
	m.fxSalt=91
	dots,other:=0,0
	for step:=uint64(0);step<180;step+=7 {
		m.fxSimSteps=step
		for _,row:=range m.snowPositions(90,30) {
			for _,cell:=range row {
				if cell.glyph=='.' {dots++} else {other++}
			}
		}
	}
	if dots==0 || other==0 {t.Fatalf("snow particle variety missing: dots=%d others=%d",dots,other)}
}


func TestSnowDotsHaveHigherVelocityForSameSeed(t *testing.T) {
	for _, seed := range []uint64{1, 11, 42, 999, 999999} {
		dot := snowflakeSpeed(seed,'.')
		flake := snowflakeSpeed(seed,'*')
		if dot <= flake || dot < 1.6 {
			t.Fatalf("seed %d: dot speed %.3f should exceed flake %.3f",seed,dot,flake)
		}
	}
}

func TestStoppingLocalEffectPreservesFallingParticles(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.width=100
	m.height=30
	m.fxLocal=chatFXState{name:"rain",started:time.Now()}
	m.fxLocalOverride=true
	m.fxSalt=19
	m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
	if m.fxLocalOverride || m.currentChatFX().name!="" {
		t.Fatal("local rain emitter should stop")
	}
	if len(m.fxFading)==0 || len(m.fxFading[0].particles)==0 {
		t.Fatal("local rain should enter a gradual particle fall-out")
	}
	m.fxDisabled=true
	rows:=[]string{strings.Repeat(" ",40),strings.Repeat(" ",40)}
	before:=append([]string(nil),rows...)
	m.renderChatFXBackground(rows,40)
	if !reflect.DeepEqual(rows,before) {t.Fatal("disablefx must hide outgoing particles as well")}
}


func TestFireworkBurstWidthsVaryAndBigShellsAreRare(t *testing.T) {
	regular := 0
	big := 0
	minWidth, maxWidth := 100.0, 0.0
	for i := 0; i < 1024; i++ {
		seed := chatFXHash(uint64(i+1)*0x9e3779b97f4a7c15+229)
		glyph, widthScale, particleScale := fireworkShellStyle(seed)
		switch glyph {
		case 'o':
			regular++
			if widthScale < 1.05 || widthScale > 2.1 || particleScale != 1 {
				t.Fatalf("invalid ordinary shell proportions: %f, %f", widthScale, particleScale)
			}
			minWidth = min(minWidth, widthScale)
			maxWidth = max(maxWidth, widthScale)
		case '0':
			big++
			if widthScale < 2.35 || particleScale <= 1 {
				t.Fatalf("rare shell should be substantially bigger: %f, %f", widthScale, particleScale)
			}
		default:
			t.Fatalf("unexpected shell glyph %q", glyph)
		}
	}
	if big < 5 || big > 90 || regular <= big {
		t.Fatalf("rare shell distribution: %d big, %d regular", big, regular)
	}
	if maxWidth-minWidth < 0.5 {
		t.Fatalf("ordinary burst width does not vary enough: %.2f..%.2f",minWidth,maxWidth)
	}
}

func TestOversizedShellRendersNeutralAndBurstsStayInBounds(t *testing.T) {
	m := newMessageInteractionTestModel(140)
	m.fxRoom = chatFXState{name:"fireworks",started:time.Now(),until:time.Now().Add(60*time.Second)}
	m.fxSalt = 229
	seenBigShell := false
	seenBigBurst := false
	for i:=0; i<78; i++ {
		seed := chatFXHash(uint64(i+1)*0x9e3779b97f4a7c15+m.fxSalt)
		glyph,_,_:=fireworkShellStyle(seed)
		if glyph!='0' { continue }
		// Look at this rare shell just after takeoff and after detonation.
		m.fxSimSteps = uint64((float64(i)*0.78+0.15)*30)
		for y,row:=range m.fireworkPositions(140,38) {
			for x,cell:=range row {
				if x<0||x>=140||y<0||y>=38 {t.Fatal("particle outside viewport")}
				if cell.glyph=='0' {
					seenBigShell=true
					if cell.color!="#ECF1F5" {t.Fatal("big shell must be neutral-colored")}
				}
			}
		}
		m.fxSimSteps=uint64((float64(i)*0.78+2.2)*30)
		for _,row:=range m.fireworkPositions(140,38) {
			for _,cell:=range row {
				if cell.glyph=='*'||cell.glyph=='#'||cell.glyph=='+' {seenBigBurst=true}
			}
		}
	}
	if !seenBigShell||!seenBigBurst {
		t.Fatalf("oversized shell missing visual stages: launch=%v burst=%v",seenBigShell,seenBigBurst)
	}
}


func TestSmallSnowDotsFallTwiceAsFast(t *testing.T) {
	for _, seed := range []uint64{1, 11, 42, 999, 999999} {
		dot, flake := snowflakeSpeed(seed, '.'), snowflakeSpeed(seed, '*')
		if math.Abs(dot/flake-2.0) > 1e-12 {
			t.Fatalf("seed %d: dot speed %.4f; flake speed %.4f", seed, dot, flake)
		}
	}
}

func TestBlizzardWindReversesSmoothly(t *testing.T) {
	for _, salt := range []uint64{0, 1, 23, 999} {
		seenLeft, seenRight := false, false
		lastPosition := blizzardWindOffset(0, salt)
		for frame := 1; frame <= 180*30; frame++ {
			seconds := float64(frame)/30
			position := blizzardWindOffset(seconds, salt)
			velocity := blizzardWindSpeed(seconds, salt)
			if velocity < -0.5 { seenLeft = true }
			if velocity > 0.5 { seenRight = true }
			if math.Abs(position-lastPosition) > 2.0 {
				t.Fatalf("wind moved discontinuously at %.2fs for salt %d", seconds, salt)
			}
			// Analytic displacement must agree with its instantaneous speed.
			if frame%30==0 {
				derivative := (blizzardWindOffset(seconds+0.001,salt)-blizzardWindOffset(seconds-0.001,salt))/0.002
				if math.Abs(velocity-derivative)>0.01 {
					t.Fatalf("wind speed mismatch: %.3f vs %.3f", velocity, derivative)
				}
			}
			lastPosition=position
		}
		if !seenLeft || !seenRight {t.Fatalf("wind never reversed strongly for salt %d",salt)}
	}
}

func TestBlizzardIsDenseBoundedAndIndependentOfFPS(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.fxRoom=chatFXState{name:"blizzard"}
	m.fxSalt=73
	for _, size:=range [][2]int{{1,1},{12,4},{80,24},{140,40}} {
		width,height:=size[0],size[1]
		for _,seconds:=range []uint64{0,1,10,28,59,121} {
			m.fxSimSteps=seconds*30
			rows:=m.blizzardPositions(width,height)
			if len(rows)!=height {t.Fatalf("blizzard viewport mismatch %dx%d",width,height)}
			for y,row:=range rows {
				for x,cell:=range row {
					if x<0 || x>=width || y<0 || y>=height || cell.color=="" {t.Fatal("out-of-bounds or colorless snow")}
				}
			}
		}
	}
	m.fxSimSteps=300
	m.fxRenderFPS=1
	low:=m.blizzardPositions(100,30)
	m.fxRenderFPS=60
	high:=m.blizzardPositions(100,30)
	if !reflect.DeepEqual(low,high) {t.Fatal("render FPS should not change blizzard positions")}
	snow:=m.snowPositions(100,30)
	blizzardCount,snowCount:=0,0
	for _,r:=range high {blizzardCount+=len(r)}
	for _,r:=range snow {snowCount+=len(r)}
	if blizzardCount <= snowCount*2 {
		t.Fatalf("blizzard not sufficiently dense: blizzard %d, snow %d",blizzardCount,snowCount)
	}
}

func TestBlizzardCommandRoomSelfAndHandoff(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.width=100
	m.height=32
	m.room="lobby"
	m.connected=true
	var wire bytes.Buffer
	m.client=&wireClient{enc:json.NewEncoder(&wire)}
	if m.sendMessage("/BLIZZARD self 20","") {t.Fatal("local blizzard became a chat message")}
	if wire.Len()!=0 || m.currentChatFX().name!="blizzard" {t.Fatal("self blizzard was not local")}
	m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
	if m.sendMessage("/blizzard 25","") {t.Fatal("room blizzard became chat text")}
	var packet Packet
	if err:=json.NewDecoder(&wire).Decode(&packet);err!=nil {t.Fatal(err)}
	if packet.Type!="fx_start" || packet.FX!="blizzard" || packet.FXSeconds!=25 {
		t.Fatalf("wrong blizzard room packet: %+v",packet)
	}
	m.fxRoom=chatFXState{name:"rain"}
	m.receiveChatFX(Packet{Type:"fx_state",FX:"blizzard"})
	if m.currentChatFX().name!="blizzard" || len(m.fxFading)==0 {
		t.Fatal("blizzard did not replace and phase out the old room weather")
	}
	rows:=[]string{"  [19:49] Alice: Hold on to your hats!",strings.Repeat(" ",60),strings.Repeat(" ",60)}
	m.renderChatFXBackground(rows,60)
	if !strings.HasPrefix(stripANSISequences(rows[0]),"  [19:49] Alice: Hold on to your hats!") {
		t.Fatal("blizzard obscured chat text")
	}
}


func TestBlizzardGustHoldsAndCalmIntervalsVary(t *testing.T) {
	minGust,maxGust:=math.Inf(1),0.0
	minCalm,maxCalm:=math.Inf(1),0.0
	mild,extreme:=false,false
	for _,salt:=range []uint64{0,1,91,999} {
		for phase:=uint64(0);phase<80;phase++ {
			p:=blizzardPhase(phase,salt)
			if p.gustDuration<20 || p.gustDuration>120 { t.Fatalf("gust %.2fs outside 20-120s",p.gustDuration) }
			if p.calmDuration<6 || p.calmDuration>35 { t.Fatalf("calm %.2fs outside 6-35s",p.calmDuration) }
			if p.rampDuration<3 || p.rampDuration>8 { t.Fatalf("ramp %.2fs outside 3-8s",p.rampDuration) }
			minGust=min(minGust,p.gustDuration)
			maxGust=max(maxGust,p.gustDuration)
			minCalm=min(minCalm,p.calmDuration)
			maxCalm=max(maxCalm,p.calmDuration)
			if math.Abs(p.speed)<10 {mild=true}
			if math.Abs(p.speed)>40 {extreme=true}
			if phase>0 {
				previous:=blizzardPhase(phase-1,salt)
				if (p.speed>0)==(previous.speed>0) {t.Fatal("wind did not alternate direction")}
			}
		}
	}
	if maxGust-minGust<60 || maxCalm-minCalm<16 || !mild || !extreme {
		t.Fatalf("gust variety inadequate gust %.1f..%.1f calm %.1f..%.1f mild=%v extreme=%v",
			minGust,maxGust,minCalm,maxCalm,mild,extreme)
	}
}

func TestBlizzardCalmAndSmoothReversal(t *testing.T) {
	for _,salt:=range []uint64{0,1,34} {
		first:=blizzardPhase(0,salt)
		second:=blizzardPhase(1,salt)
		for _,seconds:=range []float64{
			0,first.gustDuration,first.gustDuration+first.calmDuration/2,
			first.gustDuration+first.calmDuration,
			first.gustDuration+first.calmDuration+second.rampDuration,
		} {
			if math.IsNaN(blizzardWindOffset(seconds,salt)) {t.Fatal("nonfinite wind displacement")}
		}
		if math.Abs(blizzardWindSpeed(first.gustDuration+first.calmDuration/2,salt))>1e-10 {
			t.Fatal("blizzard should linger in calm between gusts")
		}
		before:=blizzardWindSpeed(first.gustDuration+first.calmDuration-0.001,salt)
		after:=blizzardWindSpeed(first.gustDuration+first.calmDuration+0.001,salt)
		if math.Abs(before-after)>0.1 {t.Fatal("wind snapped when changing direction")}
		if math.Abs(blizzardWindOffset(first.gustDuration,salt)-
			blizzardWindOffset(first.gustDuration+first.calmDuration,salt))>1e-8 {
			t.Fatal("blizzard drifted during its calm period")
		}
	}
}

func TestBlizzardGlyphPoolPrefersSnowOverSlash(t *testing.T) {
	m:=newMessageInteractionTestModel(120)
	m.fxRoom=chatFXState{name:"blizzard"}
	m.fxSalt=987
	punctuation,slash,total:=0,0,0
	for tick:=uint64(0);tick<180;tick+=5 {
		m.fxSimSteps=tick
		for _,row:=range m.blizzardPositions(120,35) {
			for _,c:=range row {
				total++
				if c.glyph==',' {punctuation++}
				if c.glyph=='/' || c.glyph=='\\' {slash++}
			}
		}
	}
	if punctuation==0 || slash*8>=total {
		t.Fatalf("snow glyphs too slash-heavy: %d slash / %d total, commas %d",slash,total,punctuation)
	}
}

func TestEmbersAreUpwardGlowingAndBounded(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.fxRoom=chatFXState{name:"embers"}
	m.fxSalt=412
	colors:=map[string]bool{}
	found:=0
	for _,size:=range [][2]int{{1,1},{15,6},{100,32},{160,44}} {
		width,height:=size[0],size[1]
		for _,seconds:=range []uint64{0,1,4,15,55,121} {
			m.fxSimSteps=seconds*30
			rows:=m.emberPositions(width,height)
			if len(rows)!=height {t.Fatalf("ember rows %d, expected %d",len(rows),height)}
			for y,row:=range rows {
				for x,cell:=range row {
					if x<0 || x>=width || y<0 || y>=height {t.Fatal("ember escaped viewport")}
					if cell.glyph!='.' && cell.glyph!='·' && cell.glyph!='+' && cell.glyph!='*' {
						t.Fatalf("unexpected ember glyph %q",cell.glyph)
					}
					colors[cell.color]=true
					found++
				}
			}
		}
	}
	if found<20 || len(colors)<3 {t.Fatalf("embers lack variety: %d sparks, %d colors",found,len(colors))}
	m.fxSimSteps=120
	m.fxRenderFPS=1
	low:=m.emberPositions(100,30)
	m.fxRenderFPS=60
	high:=m.emberPositions(100,30)
	if !reflect.DeepEqual(low,high) {t.Fatal("ember positions must not depend on FPS")}
}

func TestEmbersCommandAndFadeOut(t *testing.T) {
	m:=newMessageInteractionTestModel(95)
	m.room="lobby"
	m.connected=true
	m.width=95
	m.height=30
	var outgoing bytes.Buffer
	m.client=&wireClient{enc:json.NewEncoder(&outgoing)}
	if m.sendMessage("/EMBERS self 30","") {t.Fatal("self embers became a message")}
	if outgoing.Len()!=0 || m.currentChatFX().name!="embers" {t.Fatal("self embers sent a packet")}
	m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
	if len(m.fxFading)==0 || m.currentChatFX().name!="" {t.Fatal("embers did not fade out")}
	for _,p:=range m.fxFading[0].particles {
		if p.vy>=0 {t.Fatalf("ember should continue rising, got velocity %.2f",p.vy)}
	}
	if m.sendMessage("/embers 20","") {t.Fatal("room embers became a message")}
	var packet Packet
	if err:=json.NewDecoder(&outgoing).Decode(&packet);err!=nil {t.Fatal(err)}
	if packet.Type!="fx_start" || packet.FX!="embers" || packet.FXSeconds!=20 {
		t.Fatalf("wrong embers room packet: %+v",packet)
	}
	m.fxRoom=chatFXState{name:"snow"}
	m.receiveChatFX(Packet{Type:"fx_state",FX:"embers"})
	if m.currentChatFX().name!="embers" {t.Fatal("room embers did not replace snow")}
	rows:=[]string{"  [22:00] User: The fire is warm.",strings.Repeat(" ",60),strings.Repeat(" ",60)}
	m.renderChatFXBackground(rows,60)
	if !strings.HasPrefix(stripANSISequences(rows[0]),"  [22:00] User: The fire is warm.") {
		t.Fatal("embers obscured message")
	}
}

func BenchmarkBlizzardPositions(b *testing.B) {
	m:=newMessageInteractionTestModel(160)
	m.fxRoom=chatFXState{name:"blizzard"}
	m.fxSalt=127
	b.ReportAllocs()
	for i:=0;i<b.N;i++ {
		m.fxSimSteps=uint64(i*2)
		m.blizzardPositions(160,45)
	}
}


func TestFireAndMatrixCommandsRoomAndSelf(t *testing.T) {
	for _, name := range []string{"fire","matrix"} {
		m:=newMessageInteractionTestModel(95)
		m.width=95
		m.height=30
		m.room="lobby"
		m.connected=true
		var wire bytes.Buffer
		m.client=&wireClient{enc:json.NewEncoder(&wire)}
		if m.sendMessage("/"+name+" self 20","") {t.Fatalf("%s self sent as a message",name)}
		if wire.Len()!=0 || m.currentChatFX().name!=name {
			t.Fatalf("%s self did not stay local",name)
		}
		m.chatFXCommand("/stopfx",[]string{"/stopfx","self"})
		if len(m.fxFading)==0 {t.Fatalf("%s did not phase out when stopped",name)}
		if m.sendMessage("/"+name+" 25","") {t.Fatalf("%s became chat text",name)}
		var packet Packet
		if err:=json.NewDecoder(&wire).Decode(&packet);err!=nil {t.Fatal(err)}
		if packet.Type!="fx_start" || packet.FX!=name || packet.FXSeconds!=25 {
			t.Fatalf("%s room packet invalid: %+v",name,packet)
		}
		m.receiveChatFX(Packet{Type:"fx_state",FX:name})
		if m.currentChatFX().name!=name {t.Fatalf("%s room state missing",name)}
		rows:=[]string{"  [22:55] Alice: These letters stay readable!",strings.Repeat(" ",75),strings.Repeat(" ",75)}
		m.renderChatFXBackground(rows,75)
		if !strings.HasPrefix(stripANSISequences(rows[0]),"  [22:55] Alice: These letters stay readable!") {
			t.Fatalf("%s obscured message content",name)
		}
	}
}

func TestFireFlamesAreWarmConnectedAndAnimated(t *testing.T) {
	m:=newMessageInteractionTestModel(110)
	m.fxRoom=chatFXState{name:"fire"}
	m.fxSalt=313
	m.fxSimSteps=0
	first:=m.firePositions(110,34)
	m.fxSimSteps=180
	later:=m.firePositions(110,34)
	if reflect.DeepEqual(first,later) {t.Fatal("flame tongues do not flicker")}
	colors:=map[string]bool{}
	flameCells,lowerCells:=0,0
	for y,row:=range later {
		for x,cell:=range row {
			if x<0||x>=110||y<0||y>=34 {t.Fatal("flame outside viewport")}
			colors[cell.color]=true
			flameCells++
			if y>=24 {lowerCells++}
		}
	}
	if flameCells<110 || lowerCells<110 || len(colors)<3 {
		t.Fatalf("fire is too sparse or monochrome: %d cells, lower=%d, colors=%d",flameCells,lowerCells,len(colors))
	}
	m.fxRenderFPS=1
	low:=m.firePositions(110,34)
	m.fxRenderFPS=60
	if !reflect.DeepEqual(low,m.firePositions(110,34)) {t.Fatal("fire speed depends on FPS")}
}

func TestMatrixFallsWithBrightHeadsAndDarkTrails(t *testing.T) {
	m:=newMessageInteractionTestModel(110)
	m.fxRoom=chatFXState{name:"matrix"}
	m.fxSalt=71
	m.fxSimSteps=0
	first:=m.matrixPositions(110,36)
	m.fxSimSteps=150
	later:=m.matrixPositions(110,36)
	if reflect.DeepEqual(first,later) {t.Fatal("Matrix streams do not fall")}
	colors:=map[string]bool{}
	glyphs:=map[rune]bool{}
	cells:=0
	for y,row:=range later {
		for x,cell:=range row {
			if x<0||x>=110||y<0||y>=36 {t.Fatal("Matrix code outside viewport")}
			colors[cell.color]=true
			glyphs[cell.glyph]=true
			cells++
		}
	}
	if cells<20 || len(colors)<3 || len(glyphs)<10 {
		t.Fatalf("Matrix lacks code variety: %d cells, colors=%d, glyphs=%d",cells,len(colors),len(glyphs))
	}
	m.fxRenderFPS=1
	low:=m.matrixPositions(110,36)
	m.fxRenderFPS=60
	if !reflect.DeepEqual(low,m.matrixPositions(110,36)) {t.Fatal("Matrix speed depends on FPS")}
	for _,size:=range [][2]int{{1,1},{12,4},{80,24}} {
		rows:=m.matrixPositions(size[0],size[1])
		if len(rows)!=size[1] {t.Fatal("Matrix viewport wrong height")}
	}
}

func BenchmarkFireAndMatrixPositions(b *testing.B) {
	m:=newMessageInteractionTestModel(160)
	m.fxSalt=127
	b.ReportAllocs()
	for i:=0;i<b.N;i++ {
		m.fxSimSteps=uint64(i*2)
		m.firePositions(160,45)
		m.matrixPositions(160,45)
	}
}


func TestDoomFireUsesHeatPropagationAndASCIIFlames(t *testing.T) {
	const width,height=90,28
	m:=newMessageInteractionTestModel(width)
	m.fxRoom=chatFXState{name:"fire"}
	m.fxSalt=173
	state:=newDoomFireState(width,height,m.fxSalt)
	if state==nil || len(state.heat)!=width*state.height {
		t.Fatal("Doom fire heat grid was not initialized")
	}
	if state.height!=doomFireHeight(width,height) {
		t.Fatalf("fire simulation height %d does not match adaptive height %d",state.height,doomFireHeight(width,height))
	}
	for x:=0;x<width;x++ {
		if state.heat[(state.height-1)*width+x]<32 {
			t.Fatal("bottom fuel row is not hot")
		}
	}
	m.fxFire=state
	first:=m.firePositions(width,height)
	colored:=0
	palette:=map[string]bool{}
	glyphs:=map[rune]bool{}
	for y,row:=range first {
		for x,cell:=range row {
			if x<0||x>=width||y<0||y>=height {
				t.Fatal("Doom fire escaped full viewport")
			}
			if cell.glyph==' ' || cell.background!="" || cell.color=="" {
				t.Fatal("Doom fire must use colored foreground ASCII, not background blocks")
			}
			colored++
			palette[cell.color]=true
			glyphs[cell.glyph]=true
		}
	}
	if colored<width*3 || len(palette)<8 || len(glyphs)<5 {
		t.Fatalf("fire lacks warm gradient and varied glyphs: cells=%d colors=%d glyphs=%d",colored,len(palette),len(glyphs))
	}
	m.fxSimSteps=180
	later:=m.firePositions(width,height)
	if reflect.DeepEqual(first,later) {t.Fatal("Doom fire is not propagating")}
	if state.step!=m.fxSimSteps {t.Fatal("heat simulation did not follow the fixed clock")}
	m.fxRenderFPS=1
	low:=m.firePositions(width,height)
	m.fxRenderFPS=60
	if !reflect.DeepEqual(low,m.firePositions(width,height)) {
		t.Fatal("Doom fire physics must not depend on render FPS")
	}
}

func TestDoomFireResizesAndKeepsTextReadable(t *testing.T) {
	m:=newMessageInteractionTestModel(90)
	m.fxSalt=32
	m.fxRoom=chatFXState{name:"fire"}
	m.width=90
	m.height=30
	m.resetChatFXSimulation()
	if m.fxFire==nil {t.Fatal("fire should initialize on FX start")}
	for _,size:=range [][2]int{{1,1},{12,5},{90,26},{150,42}} {
		rows:=m.firePositions(size[0],size[1])
		if len(rows)!=size[1] {t.Fatal("fire height did not adapt on resize")}
		for y,row:=range rows {
			for x:=range row {
				if x<0||x>=size[0]||y<0||y>=size[1] {
					t.Fatal("fire particle outside viewport")
				}
			}
		}
	}
	lines:=[]string{
		"  [12:00] Alice: Keep this message readable!",
		strings.Repeat(" ",70),
		strings.Repeat(" ",70),
		strings.Repeat(" ",70),
	}
	m.renderChatFXBackground(lines,70)
	if !strings.HasPrefix(stripANSISequences(lines[0]),"  [12:00] Alice: Keep this message readable!") {
		t.Fatal("Doom fire background obscured chat text")
	}
}

func BenchmarkDoomFireHeatPropagation(b *testing.B) {
	state:=newDoomFireState(160,45,197)
	b.ReportAllocs()
	for i:=0;i<b.N;i++ {state.spread(197,uint64(i))}
}


func TestDoomFireUsesResponsiveHeightInsteadOfFlatSeventyPercent(t *testing.T) {
	for _,height:=range []int{10,22,36,60} {
		state:=newDoomFireState(100,height,483)
		if state.height!=doomFireHeight(100,height) || len(state.heat)!=100*state.height {
			t.Fatalf("fire heat grid sized incorrectly at height=%d: grid %dx%d",height,state.width,state.height)
		}
		// Natural cooling within the responsive budget determines the flame tips.
		for i:=uint64(0);i<140;i++ {state.spread(483,i+100)}
		if state.heat[(state.height-1)*state.width+30]==0 {
			t.Fatal("fire fuel went out during steady burning")
		}
	}
}

func TestDoomFireHeatMapsToProgressivelyFinerEmbers(t *testing.T) {
	allowed:=map[rune]bool{'@':true,'#':true,'%':true,'&':true,'*':true,
		'o':true,'x':true,'+':true,';':true,':':true,'.':true,',':true}
	for level:=uint8(1);level<=36;level++ {
		glyph:=doomFireGlyph(level,uint64(level))
		if !allowed[glyph] {t.Fatalf("unexpected fire glyph %q at heat %d",glyph,level)}
		if doomFirePalette[level]=="" {t.Fatalf("missing ember palette shade for heat %d",level)}
	}
	for _,glyph:=range []rune{'@','#','%','&'} {
		found:=false
		for i:=uint64(0);i<24;i++ {if doomFireGlyph(36,i)==glyph {found=true;break}}
		if !found {t.Fatalf("large hot glyph %q missing",glyph)}
	}
	for _,glyph:=range []rune{'.',','} {
		found:=false
		for i:=uint64(0);i<24;i++ {if doomFireGlyph(2,i)==glyph {found=true;break}}
		if !found {t.Fatalf("fine ember glyph %q missing",glyph)}
	}
}


func TestDoomFireKeepsGentleSevenPointFiveHzCadence(t *testing.T) {
	state:=newDoomFireState(80,26,313)
	original:=append([]uint8(nil),state.heat...)
	state.advance(3,313)
	if !reflect.DeepEqual(original,state.heat) {
		t.Fatal("fire propagated before fourth 30 Hz simulation tick")
	}
	state.advance(4,313)
	if reflect.DeepEqual(original,state.heat) {
		t.Fatal("fire did not propagate on fourth 30 Hz simulation tick")
	}
	settled:=append([]uint8(nil),state.heat...)
	state.advance(7,313)
	if !reflect.DeepEqual(settled,state.heat) {
		t.Fatal("fire propagated between scheduled slow heat updates")
	}
	state.advance(8,313)
	if reflect.DeepEqual(settled,state.heat) {
		t.Fatal("fire did not propagate on next scheduled update")
	}
}

func TestDoomFireRendersWarmFlamesWithoutFaintSparkCloud(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.fxRoom=chatFXState{name:"fire"}
	m.fxSalt=281
	state:=newDoomFireState(100,34,m.fxSalt)
	m.fxFire=state
	// A weak heat cell is simulated, but not rendered as endless fine
	// speckle over the top of the terminal.
	state.heat[0]=4
	rows:=m.firePositions(100,34)
	if _,visible:=rows[0][0];visible {
		t.Fatal("dying heat should disappear rather than flicker as a stray dot")
	}
	for _,row:=range rows {
		for _,cell:=range row {
			if cell.background!="" {t.Fatal("fire unexpectedly changed chat background")}
			if cell.color=="" || cell.glyph==' ' {t.Fatal("fire lost its warm colored glyphs")}
		}
	}
}


func TestDoomFireBaseIsDenseAndGustsReverse(t *testing.T) {
	const width,height=100,34
	state:=newDoomFireState(width,height,719)
	for i:=uint64(0);i<120;i++ {state.spread(719,i+200)}
	for y:=state.height-4;y<state.height;y++ {
		dense:=0
		for x:=0;x<width;x++ {
			if state.heat[y*width+x]>=20 {dense++}
		}
		if dense<width*8/10 {
			t.Fatalf("fire base row %d is too patchy: %d/%d hot cells",y,dense,width)
		}
	}
	positive,negative:=false,false
	for tick:=uint64(0);tick<240;tick++ {
		wind:=fireWind(tick,719)
		if wind>0.5 {positive=true}
		if wind< -0.5 {negative=true}
		if tick>0 && math.Abs(wind-fireWind(tick-1,719))>0.07 {
			t.Fatal("flame wind changed abruptly")
		}
	}
	if !positive || !negative {t.Fatal("fire wind failed to reverse direction")}
}

func TestDoomFireBatchedRendererPreservesMessages(t *testing.T) {
	const width,height=100,22
	m:=newMessageInteractionTestModel(width)
	m.fxRoom=chatFXState{name:"fire"}
	m.fxSalt=929
	m.fxFire=newDoomFireState(width,height,m.fxSalt)
	rows:=make([]string,height)
	for i:=range rows {rows[i]=strings.Repeat(" ",width)}
	message:="  [23:16] Alice: Nice cozy fire."
	rows[height-2]=message+strings.Repeat(" ",width-len(message))
	m.renderDoomFireRows(rows,width)
	if !strings.HasPrefix(stripANSISequences(rows[height-2]),message) {
		t.Fatal("batched fire renderer overwrote message")
	}
	// A contiguous row of one heat shade must be one foreground run,
	// not a separate styled fragment for every single character.
	for x:=range m.fxFire.heat[(height-1)*width:] {
		m.fxFire.heat[(height-1)*width+x]=36
	}
	blank:=[]string{strings.Repeat(" ",width)}
	m.fxFire.height=1
	m.fxFire.heat=m.fxFire.heat[(height-1)*width:]
	m.renderDoomFireRows(blank,width)
	if strings.Count(blank[0],"\x1b[")>=width/4 {
		t.Fatalf("batched fire emitted too many ANSI fragments: %d",strings.Count(blank[0],"\x1b["))
	}
}

func BenchmarkDoomFireBatchedRenderer(b *testing.B) {
	m:=newMessageInteractionTestModel(160)
	m.fxRoom=chatFXState{name:"fire"}
	m.fxSalt=991
	m.fxFire=newDoomFireState(160,45,m.fxSalt)
	base:=make([]string,45)
	for i:=range base {base[i]=strings.Repeat(" ",160)}
	b.ReportAllocs()
	for i:=0;i<b.N;i++ {
		m.fxSimSteps=uint64(i)
		rows:=append([]string(nil),base...)
		m.renderDoomFireRows(rows,160)
	}
}


func TestDoomFireHeightShrinksForWideTerminals(t *testing.T) {
	const viewport=45
	narrow:=doomFireHeight(80,viewport)
	medium:=doomFireHeight(160,viewport)
	wide:=doomFireHeight(320,viewport)
	if !(narrow>medium && medium>wide && wide>=4) {
		t.Fatalf("fire height should shrink with width: 80=%d, 160=%d, 320=%d",narrow,medium,wide)
	}
	if medium>=viewport/2 {
		t.Fatalf("normal terminal flames should be shorter than half the chat height: %d of %d",medium,viewport)
	}
	for _,size:=range [][2]int{{1,1},{12,4},{100,25},{300,60}} {
		width,height:=size[0],size[1]
		state:=newDoomFireState(width,height,17)
		if state.height!=doomFireHeight(width,height) {
			t.Fatalf("grid and height diverged at %dx%d",width,height)
		}
		if len(state.heat)!=width*state.height {
			t.Fatal("heat allocation exceeds flame height budget")
		}
	}
}

func TestDoomFireRendererKeepsUpperChatRowsUntouched(t *testing.T) {
	m:=newMessageInteractionTestModel(240)
	m.fxRoom=chatFXState{name:"fire"}
	m.fxSalt=37
	height,width:=40,240
	m.fxFire=newDoomFireState(width,height,m.fxSalt)
	rows:=make([]string,height)
	for i:=range rows {rows[i]=strings.Repeat(" ",width)}
	rows[0]="  [10:15] Someone: This message should be untouched."
	original:=append([]string(nil),rows...)
	m.renderDoomFireRows(rows,width)
	cutoff:=height-doomFireHeight(width,height)
	for y:=0;y<cutoff;y++ {
		if rows[y]!=original[y] {
			t.Fatalf("fire touched row %d above flame budget",y)
		}
	}
}


func BenchmarkDoomFireResponsiveRendering(b *testing.B) {
	for _,width:=range []int{80,160,320} {
		b.Run(fmt.Sprintf("%d_columns",width),func(b *testing.B) {
			const height=45
			m:=newMessageInteractionTestModel(width)
			m.fxRoom=chatFXState{name:"fire"}
			m.fxSalt=991
			m.fxFire=newDoomFireState(width,height,m.fxSalt)
			base:=make([]string,height)
			for y:=range base {base[y]=strings.Repeat(" ",width)}
			b.ReportAllocs()
			b.ResetTimer()
			for i:=0;i<b.N;i++ {
				m.fxSimSteps=uint64(i)
				rows:=append([]string(nil),base...)
				m.renderDoomFireRows(rows,width)
			}
		})
	}
}
