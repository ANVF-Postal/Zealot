package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestWhisperIsDeliveredOnlyToSenderAndRecipient(t *testing.T) {
	s := newServer()
	room := s.room("quiet", true)
	var senderData, targetData, bystanderData bytes.Buffer
	sender := &serverClient{
		id: "sender", room: "quiet",
		user: User{Name: "Postal", Color: "#ffaa44"},
		enc: json.NewEncoder(&senderData),
	}
	target := &serverClient{
		id: "target", room: "quiet",
		user: User{Name: "Alice", Color: "#ff66aa"},
		enc: json.NewEncoder(&targetData),
	}
	bystander := &serverClient{
		id: "third", room: "quiet",
		user: User{Name: "Bystander"},
		enc: json.NewEncoder(&bystanderData),
	}
	room.clients[sender.id] = sender
	room.clients[target.id] = target
	room.clients[bystander.id] = bystander

	if err := s.sendWhisper("quiet", sender, "@aLiCe", "A secret for Alice.", ""); err != nil {
		t.Fatalf("whisper send: %v", err)
	}
	if bystanderData.Len() != 0 {
		t.Fatal("whisper was leaked to a third room member")
	}
	if len(room.messages) != 0 {
		t.Fatal("whisper was saved to public room history")
	}
	var senderPacket, targetPacket Packet
	if err := json.NewDecoder(&senderData).Decode(&senderPacket); err != nil { t.Fatal(err) }
	if err := json.NewDecoder(&targetData).Decode(&targetPacket); err != nil { t.Fatal(err) }
	if senderPacket.ID == "" || senderPacket.ID != targetPacket.ID ||
		senderPacket.Type != "whisper_result" || targetPacket.Type != "whisper_result" {
		t.Fatalf("sender and recipient did not see the same private packet: %+v %+v", senderPacket, targetPacket)
	}
	if senderPacket.User.Name != "Postal" || senderPacket.WhisperTo != "Alice" ||
		senderPacket.Text != "A secret for Alice." {
		t.Fatalf("whisper used untrusted metadata: %+v", senderPacket)
	}
	if senderPacket.Time.IsZero() { t.Fatal("whisper has no daemon timestamp") }
}

func TestWhisperRejectsMissingOrInvalidRecipientAndBody(t *testing.T) {
	s := newServer()
	room := s.room("quiet", true)
	var senderData bytes.Buffer
	sender := &serverClient{
		id: "sender", room: "quiet", user: User{Name: "Postal"},
		enc: json.NewEncoder(&senderData),
	}
	room.clients[sender.id] = sender
	for _, tc := range []struct{ target, body string }{
		{"", "hello"}, {"postal", "hello"}, {"@", "hello"},
		{"@nobody", "hello"}, {"@Postal", "   "},
		{"@Postal", strings.Repeat("z", maxMessage+1)},
		{"@Postal another", "hello"},
	} {
		if err := s.sendWhisper("quiet", sender, tc.target, tc.body, ""); err == nil {
			t.Errorf("allowed whisper with target %q and body length %d", tc.target, len(tc.body))
		}
	}
	if senderData.Len() != 0 || len(room.messages) != 0 {
		t.Fatal("invalid whisper delivered a packet or was saved to history")
	}
	if err := s.sendWhisper("quiet", sender, "@postal", "whisper to myself", ""); err != nil { t.Fatal(err) }
	if got := bytes.Count(senderData.Bytes(), []byte(`"type":"whisper_result"`)); got != 1 {
		t.Fatalf("self whisper delivered %d duplicate packets", got)
	}
}

func TestWhisperChatRenderingIsDistinctAndWrapped(t *testing.T) {
	m := newMessageInteractionTestModel(85)
	packet := Packet{
		ID: "private-1", Type: "whisper_result", Room: "quiet",
		User: User{Name: "Postal", Color: "#ff3399"},
		WhisperTo: "Alice", Text: strings.Repeat("secret words ", 20),
		Time: time.Now().UTC(),
	}
	lines := m.renderPacketBaseLines(packet, m.width)
	plain := stripANSISequences(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "[WHISPER]") ||
		!strings.Contains(plain, "Postal → @Alice: ") ||
		len(lines) < 2 {
		t.Fatalf("whisper lost label/recipient/wrapping: %q", plain)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > chatContentWidth(m.width) {
			t.Fatalf("whisper overflowed narrow chat column: width=%d", lipgloss.Width(line))
		}
	}
	result := m.renderPacketBaseLines(packet, 85)
	rendered := strings.Join(result,"\n")
	bg := m.messageBackground(packet)
	usernameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(packet.User.Color)).Background(bg).Bold(true)
	if !strings.Contains(rendered, usernameStyle.Render("Postal")) {
		t.Fatalf("sender's color missing from private message: %q", rendered)
	}
}

func TestClientCannotForgeWhisperResultPacket(t *testing.T) {
	s := newServer()
	serverConn, peerConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		s.handle(serverConn)
		close(done)
	}()
	defer peerConn.Close()
	enc := json.NewEncoder(peerConn)
	dec := json.NewDecoder(bufio.NewReader(peerConn))
	if err := peerConn.SetReadDeadline(time.Now().Add(3*time.Second)); err != nil { t.Fatal(err) }
	if err := enc.Encode(Packet{Type: "hello", Room: "quiet", User: User{Name: "Postal"}}); err != nil { t.Fatal(err) }
	for {
		var p Packet
		if err := dec.Decode(&p); err != nil { t.Fatal(err) }
		if p.Type == "users" { break }
	}
	if err := enc.Encode(Packet{
		Type: "whisper_result", Room: "quiet", WhisperTo: "Postal",
		User: User{Name: "Impostor"}, Text: "forged private text",
	}); err != nil { t.Fatal(err) }
	if err := peerConn.SetReadDeadline(time.Now().Add(100*time.Millisecond)); err != nil { t.Fatal(err) }
	var got Packet
	if err := dec.Decode(&got); err == nil {
		t.Fatalf("daemon accepted forged private result: %+v", got)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("expected silence after spoofed result, got %v", err)
	}
	_ = peerConn.Close()
	select {
	case <-done:
	case <-time.After(time.Second): t.Fatal("daemon did not stop after disconnect")
	}
}

func TestWhisperDoesNotReachOtherRooms(t *testing.T) {
	s := newServer()
	origin := s.room("origin", true)
	other := s.room("other", true)
	var senderData, otherData bytes.Buffer
	sender := &serverClient{
		id: "sender", room: "origin", user: User{Name: "Postal"},
		enc: json.NewEncoder(&senderData),
	}
	stranger := &serverClient{
		id: "stranger", room: "other", user: User{Name: "Alice"},
		enc: json.NewEncoder(&otherData),
	}
	origin.clients[sender.id] = sender
	other.clients[stranger.id] = stranger
	if err := s.sendWhisper("origin", sender, "@Alice", "secret", ""); err == nil {
		t.Fatal("daemon resolved a whisper target from a different room")
	}
	if senderData.Len() != 0 || otherData.Len() != 0 {
		t.Fatal("failed cross-room whisper leaked its content")
	}
	if len(origin.messages) != 0 || len(other.messages) != 0 {
		t.Fatal("failed cross-room whisper entered room history")
	}
}

func TestWhisperRequestUsesDaemonIdentityEndToEnd(t *testing.T) {
	s := newServer()
	serverConn, peerConn := net.Pipe()
	done := make(chan struct{})
	go func() { s.handle(serverConn); close(done) }()
	defer peerConn.Close()
	enc := json.NewEncoder(peerConn)
	dec := json.NewDecoder(bufio.NewReader(peerConn))
	if err := peerConn.SetReadDeadline(time.Now().Add(3*time.Second)); err != nil { t.Fatal(err) }
	if err := enc.Encode(Packet{Type: "hello", Room: "quiet", User: User{Name: "Postal"}}); err != nil { t.Fatal(err) }
	for {
		var p Packet
		if err := dec.Decode(&p); err != nil { t.Fatal(err) }
		if p.Type == "users" { break }
	}
	// A self-whisper exercises the full protocol with one connected client,
	// avoiding unrelated sockets while checking authoritative metadata.
	if err := enc.Encode(Packet{
		Type: "whisper", Room: "some-other-room", WhisperTo: "@postal",
		User: User{Name: "Impostor", Color: "#ff0000"}, Text: "Private test",
	}); err != nil { t.Fatal(err) }
	var result Packet
	if err := dec.Decode(&result); err != nil { t.Fatal(err) }
	if result.Type != "whisper_result" || result.Room != "quiet" ||
		result.User.Name != "Postal" || result.WhisperTo != "Postal" ||
		result.Text != "Private test" {
		t.Fatalf("daemon trusted forged whisper metadata: %+v", result)
	}
	s.mu.Lock()
	history := append([]Packet(nil), s.rooms["quiet"].messages...)
	s.mu.Unlock()
	for _, item := range history {
		if item.Type == "whisper_result" || item.Text == "Private test" {
			t.Fatalf("private whisper was saved to room history: %+v", item)
		}
	}
	_ = peerConn.Close()
	select {
	case <-done:
	case <-time.After(time.Second): t.Fatal("daemon did not stop after disconnect")
	}
}
