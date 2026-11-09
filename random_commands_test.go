package main

import (
	"bufio"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestRandomCommandRangeValidation(t *testing.T) {
	valid := []struct {
		input string
		min, max int64
	}{
		{"1-100", 1, 100},
		{"0-0", 0, 0},
		{"7-7", 7, 7},
		{"999999999-1000000000", 999999999, 1000000000},
	}
	for _, tc := range valid {
		min, max, err := parseRandomRange(tc.input)
		if err != nil || min != tc.min || max != tc.max {
			t.Errorf("parseRandomRange(%q) = %d, %d, %v", tc.input, min, max, err)
		}
	}
	for _, input := range []string{
		"", "1", "1-", "-10", "10-1", "1-2-3",
		"abc-def", "1- 4", "1.0-4", "-1-5", "0-1000000001",
		"999999999999999999999999-4", "1-999999999999999999999",
	} {
		if _, _, err := parseRandomRange(input); err == nil {
			t.Errorf("parseRandomRange(%q) unexpectedly succeeded", input)
		}
	}
}

func TestSecureRandomBetweenInclusive(t *testing.T) {
	for i := 0; i < 100; i++ {
		n, err := secureRandomBetween(0, 1)
		if err != nil || n < 0 || n > 1 {
			t.Fatalf("coin result = %d, %v", n, err)
		}
		n, err = secureRandomBetween(11, 11)
		if err != nil || n != 11 {
			t.Fatalf("fixed random range = %d, %v", n, err)
		}
		n, err = secureRandomBetween(1, 1000000000)
		if err != nil || n < 1 || n > 1000000000 {
			t.Fatalf("large random result = %d, %v", n, err)
		}
	}
}

func TestAuthoritativeGamePacketsAndRendering(t *testing.T) {
	user := User{Name: "Example", Color: "#ffffff"}
	coin, err := newCoinflipResult("lobby", user)
	if err != nil || coin.Type != "coinflip_result" || coin.ID == "" ||
		(coin.Text != "Heads" && coin.Text != "Tails") || coin.User.Name != user.Name {
		t.Fatalf("invalid coin result: %#v, %v", coin, err)
	}
	number, err := newRandomResult("lobby", user, "5-5")
	if err != nil || number.Type != "random_result" || number.ID == "" ||
		number.Text != "5–5: 5" || number.User.Name != user.Name {
		t.Fatalf("invalid random result: %#v, %v", number, err)
	}
	if _, err := newRandomResult("lobby", user, "10-1"); err == nil {
		t.Fatal("daemon accepted invalid requested range")
	}
	m := newMessageInteractionTestModel(100)
	for _, tc := range []struct{
		packet Packet
		want string
	}{
		{coin, "[COIN FLIP] Example flipped a coin: " + coin.Text},
		{number, "[RANDOM] Example rolled 5–5: 5"},
	} {
		lines := m.renderPacketBaseLines(tc.packet, m.width)
		display := stripANSISequences(strings.Join(lines, "\n"))
		if !strings.Contains(display, tc.want) {
			t.Errorf("special packet did not show distinctive result: %q", display)
		}
	}
}

// Exercise the real server handler with an in-memory socket. A client can
// request a draw, but submitting a result type directly must be ignored.
func TestGameResultsCannotBeForgedByClient(t *testing.T) {
	s := newServer()
	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()
	done := make(chan struct{})
	go func() {
		s.handle(serverConn)
		close(done)
	}()
	enc := json.NewEncoder(peerConn)
	dec := json.NewDecoder(bufio.NewReader(peerConn))
	if err := enc.Encode(Packet{Type:"hello", Room:"lobby", User:User{Name:"Alice"}}); err != nil {
		t.Fatal(err)
	}
	if err := peerConn.SetReadDeadline(time.Now().Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	// The initial handshake ends with the authoritative roster broadcast.
	for {
		var packet Packet
		if err := dec.Decode(&packet); err != nil { t.Fatal(err) }
		if packet.Type == "users" { break }
	}
	if err := enc.Encode(Packet{Type:"coinflip", Text:"Tails", User:User{Name:"Impostor"}}); err != nil {
		t.Fatal(err)
	}
	var result Packet
	for {
		if err := dec.Decode(&result); err != nil { t.Fatal(err) }
		if result.Type == "coinflip_result" { break }
	}
	if result.User.Name != "Alice" || (result.Text != "Heads" && result.Text != "Tails") {
		t.Fatalf("daemon used untrusted identity or coin text: %#v",result)
	}
	// Drain the remaining typing + roster updates from the first draw.
	for {
		var p Packet
		if err := dec.Decode(&p); err != nil { t.Fatal(err) }
		if p.Type == "users" { break }
	}
	if err := enc.Encode(Packet{Type:"random_result", Text:"1–100: 100", User:User{Name:"Impostor"}}); err != nil {
		t.Fatal(err)
	}
	if err := peerConn.SetReadDeadline(time.Now().Add(100*time.Millisecond)); err != nil { t.Fatal(err) }
	var forged Packet
	if err := dec.Decode(&forged); err == nil {
		t.Fatalf("daemon broadcast a client-forged result: %#v",forged)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("expected silence after forged result, got %v",err)
	}
	_ = peerConn.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("daemon handler did not stop after disconnect")
	}
}

func TestRandomResultNumberIsWithinRequestedRange(t *testing.T) {
	for i:=0; i<30; i++ {
		p, err := newRandomResult("lobby", User{Name:"Alice"}, "200-250")
		if err!=nil { t.Fatal(err) }
		chunks := strings.Split(p.Text, ": ")
		if len(chunks)!=2 { t.Fatalf("malformed result: %q",p.Text) }
		n, err := strconv.ParseInt(chunks[1],10,64)
		if err!=nil || n<200 || n>250 { t.Fatalf("out of range: %q (%v)",p.Text,err) }
	}
}

func TestGameResultColorsFollowUsernameAndTheme(t *testing.T) {
	m := newMessageInteractionTestModel(120)
	user := User{Name: "Postal", Color: "#FF3399"}
	cases := []struct {
		kind, body, action, outcome, label string
	}{
		{"coinflip_result", "Heads", "Postal flipped a coin: ", "Heads", "[COIN FLIP]"},
		{"random_result", "1–100: 42", "Postal rolled 1–100: ", "42", "[RANDOM]"},
	}
	bg := m.messageBackground(Packet{Type: "coinflip_result", User: user})
	userStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(user.Color)).Background(bg)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.TextColor)).Background(bg)
	for _, tc := range cases {
		packet := Packet{Type: tc.kind, User: user, Text: tc.body, Time: time.Now().UTC()}
		rows := m.renderPacketBaseLines(packet, m.width)
		rendered := strings.Join(rows, "\n")
		for _, segment := range []string{
			m.styles.accent.Render(tc.label),
			userStyle.Render(tc.action),
			normalStyle.Render(tc.outcome),
		} {
			if !strings.Contains(rendered, segment) {
				t.Errorf("%s missing styled segment %q in %q", tc.kind, segment, rendered)
			}
		}
		plain := stripANSISequences(rendered)
		if !strings.Contains(plain, tc.action+tc.outcome) {
			t.Errorf("%s lost its content: %q", tc.kind, plain)
		}
	}
}
