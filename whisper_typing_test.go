package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// Read all packets produced by the local client without connecting sockets.
func decodeTypingPackets(t *testing.T, buffer *bytes.Buffer) []Packet {
	t.Helper()
	var packets []Packet
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	for decoder.More() {
		var packet Packet
		if err := decoder.Decode(&packet); err != nil {
			t.Fatal(err)
		}
		packets = append(packets, packet)
	}
	buffer.Reset()
	return packets
}

func TestWhisperTypingWaitsForExactUsernameAndClearsPublicState(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.room = "test"
	m.user = User{Name:"Alice", Color:"#ff3399"}
	m.onlineUsers = []User{{Name:"Daniel2",Color:"#11eeaa"},{Name:"Don",Color:"#aa11ee"}}
	var stream bytes.Buffer
	m.client = &wireClient{enc:json.NewEncoder(&stream)}
	m.input.SetValue("hello room")
	m.syncTypingState()
	if packets := decodeTypingPackets(t,&stream);len(packets)!=1 || packets[0].Type!="typing" || !packets[0].Typing {
		t.Fatalf("regular text did not send public typing: %+v",packets)
	}
	m.input.SetValue("/whisper")
	m.syncTypingState()
	if packets:=decodeTypingPackets(t,&stream);len(packets)!=1 || packets[0].Type!="typing" || packets[0].Typing {
		t.Fatalf("whisper command did not immediately stop public typing: %+v",packets)
	}
	for _, draft := range []string{"/whisper @d","/whisper @da","/whisper @daniel","/whisper @daniel2x"} {
		m.input.SetValue(draft)
		m.syncTypingState()
		if packets:=decodeTypingPackets(t,&stream);len(packets)!=0 {
			t.Fatalf("partial username %q notified a recipient: %+v",draft,packets)
		}
	}
	m.input.SetValue("/whisper @daniel2")
	m.syncTypingState()
	if packets:=decodeTypingPackets(t,&stream);len(packets)!=1 || packets[0].Type!="whisper_typing" || !packets[0].Typing || packets[0].WhisperTo!="@Daniel2" {
		t.Fatalf("full username did not start private typing: %+v",packets)
	}
	m.input.SetValue("/whisper @d")
	m.syncTypingState()
	if packets:=decodeTypingPackets(t,&stream);len(packets)!=1 || packets[0].Type!="whisper_typing" || packets[0].Typing {
		t.Fatalf("shortened username did not clear private typing: %+v",packets)
	}
	m.input.SetValue("public again")
	m.syncTypingState()
	if packets:=decodeTypingPackets(t,&stream);len(packets)!=1 || packets[0].Type!="typing" || !packets[0].Typing {
		t.Fatalf("switch back to public typing failed: %+v",packets)
	}
}

func TestWhisperTypingSwitchesRecipientsAndRepliesStayPrivate(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	m.user=User{Name:"Postal"}
	m.room="test"
	m.onlineUsers=[]User{{Name:"Daniel1"},{Name:"Daniel2"}}
	var stream bytes.Buffer
	m.client=&wireClient{enc:json.NewEncoder(&stream)}
	m.input.SetValue("/whisper @Daniel1 ")
	m.syncTypingState()
	decodeTypingPackets(t,&stream)
	m.input.SetValue("/whisper @Daniel2 hello")
	m.syncTypingState()
	packets:=decodeTypingPackets(t,&stream)
	if len(packets)!=2 || packets[0].Type!="whisper_typing" || packets[0].Typing ||
		packets[0].WhisperTo!="@Daniel1" || packets[1].Type!="whisper_typing" ||
		!packets[1].Typing || packets[1].WhisperTo!="@Daniel2" {
		t.Fatalf("did not stop old recipient before starting new: %+v",packets)
	}
	parent:=whisperReplyTestPacket("private-parent","Daniel1","Postal","hello")
	m.appendMessage(parent)
	m.beginReply(parent.ID)
	m.input.SetValue("private answer")
	m.syncTypingState()
	packets=decodeTypingPackets(t,&stream)
	if len(packets)!=2 || packets[0].WhisperTo!="@Daniel2" || packets[0].Typing ||
		packets[1].WhisperTo!="@Daniel1" || !packets[1].Typing {
		t.Fatalf("whisper reply did not switch private typing to original sender: %+v",packets)
	}
}

func TestPrivateTypingIndicatorUsesSenderColor(t *testing.T) {
	m:=newMessageInteractionTestModel(100)
	user:=User{Name:"Daniel1",Color:"#FF3399"}
	updated,_:=m.Update(messageReceivedMsg(Packet{
		Type:"whisper_typing",Room:"test",User:user,Typing:true,
	}))
	m=updated.(model)
	if len(m.privateTyping)!=1 || len(m.typing)!=0 {
		t.Fatalf("private event leaked into room-wide typing state")
	}
	rendered:=m.View().Content
	plain:=stripANSISequences(rendered)
	if !strings.Contains(plain,"Daniel1 is whispering to you") {
		t.Fatalf("private indicator missing from chat: %q",plain)
	}
	style:=lipgloss.NewStyle().Foreground(lipgloss.Color(user.Color)).
		Background(lipgloss.Color(m.cfg.Theme.Background)).
		Width(chatContentWidth(m.width))
	if !strings.Contains(rendered,style.Render("  Daniel1 is whispering to you…")) {
		t.Fatal("private indicator was not styled using sender's username color")
	}
	updated,_=m.Update(messageReceivedMsg(Packet{
		Type:"whisper_typing",Room:"test",User:user,Typing:false,
	}))
	m=updated.(model)
	if len(m.privateTyping)!=0 || strings.Contains(stripANSISequences(m.View().Content),"is whispering to you") {
		t.Fatal("private typing indicator remained after stop")
	}
}

func TestDaemonWhisperTypingOnlyReachesVerifiedRecipient(t *testing.T) {
	s:=newServer()
	room:=s.room("test",true)
	var sourceData, targetData, bystanderData bytes.Buffer
	sender:=&serverClient{id:"sender",room:"test",user:User{Name:"Daniel1",Color:"#ff3399"},enc:json.NewEncoder(&sourceData)}
	target:=&serverClient{id:"recipient",room:"test",user:User{Name:"Daniel2",Color:"#33ff99"},enc:json.NewEncoder(&targetData)}
	third:=&serverClient{id:"bystander",room:"test",user:User{Name:"Don"},enc:json.NewEncoder(&bystanderData)}
	room.clients[sender.id]=sender
	room.clients[target.id]=target
	room.clients[third.id]=third

	s.setWhisperTyping("test",sender,"@dan",true)
	if targetData.Len()!=0 || bystanderData.Len()!=0 {
		t.Fatal("daemon delivered typing to prefix match")
	}
	s.setWhisperTyping("test",sender,"@Daniel2",true)
	packets:=decodeTypingPackets(t,&targetData)
	if len(packets)!=1 || packets[0].Type!="whisper_typing" || !packets[0].Typing ||
		packets[0].User.Name!="Daniel1" || packets[0].User.Color!="#ff3399" {
		t.Fatalf("private typing metadata invalid: %+v",packets)
	}
	if bystanderData.Len()!=0 || sourceData.Len()!=0 {
		t.Fatal("recipient-only typing leaked to sender or bystander")
	}
	if len(room.messages)!=0 || sender.typing || sender.whisperTarget!="Daniel2" {
		t.Fatal("private typing incorrectly entered public history or roster")
	}
	s.setWhisperTyping("test",sender,"",false)
	packets=decodeTypingPackets(t,&targetData)
	if len(packets)!=1 || packets[0].Type!="whisper_typing" || packets[0].Typing {
		t.Fatalf("stop whisper typing not delivered: %+v",packets)
	}
}

func TestDaemonSwitchesPublicTypingOffWhenWhisperStarts(t *testing.T) {
	s:=newServer()
	room:=s.room("test",true)
	var sourceData,targetData,thirdData bytes.Buffer
	sender:=&serverClient{id:"sender",room:"test",user:User{Name:"Daniel1",Color:"#ff3399"},typing:true,enc:json.NewEncoder(&sourceData)}
	target:=&serverClient{id:"target",room:"test",user:User{Name:"Daniel2"},enc:json.NewEncoder(&targetData)}
	third:=&serverClient{id:"third",room:"test",user:User{Name:"Don"},enc:json.NewEncoder(&thirdData)}
	room.clients[sender.id]=sender
	room.clients[target.id]=target
	room.clients[third.id]=third
	s.setWhisperTyping("test",sender,"@Daniel2",true)
	for _, stream := range []*bytes.Buffer{&sourceData,&thirdData} {
		packets:=decodeTypingPackets(t,stream)
		stopped:=false
		for _,packet:=range packets {
			if packet.Type=="whisper_typing" {t.Fatal("whisper typing leaked to the room")}
			if packet.Type=="typing" && !packet.Typing && packet.User.Name=="Daniel1" {stopped=true}
		}
		if !stopped {t.Fatal("public typing did not stop on whisper transition")}
	}
	targetPackets:=decodeTypingPackets(t,&targetData)
	privateStarted:=false
	for _,packet:=range targetPackets {
		if packet.Type=="whisper_typing" && packet.Typing && packet.User.Name=="Daniel1" {
			privateStarted=true
		}
	}
	if !privateStarted || sender.typing || sender.whisperTarget!="Daniel2" {
		t.Fatalf("private typing never started: packets=%+v",targetPackets)
	}
}
