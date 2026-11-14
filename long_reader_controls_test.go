package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestLongReaderUsesNormalChatWordWrap(t *testing.T) {
	packet := interactionTestPacket("reader-wrap", "First sentence has multiple words that should wrap cleanly at word boundaries.\n\nThe second paragraph also has words.")
	packet.Long = true
	for _, width := range []int{35, 80} {
		m := newMessageInteractionTestModel(width, packet)
		m.openLongReader(packet)
		want := wrapText(packet.Text, max(1, chatContentWidth(width)-1))
		if len(want) != len(m.longReaderLines) {
			t.Fatalf("width %d: reader has %d rows, normal wrapper gives %d", width, len(m.longReaderLines), len(want))
		}
		for i := range want {
			if m.longReaderLines[i] != want[i] {
				t.Fatalf("width %d row %d: got %q, want %q", width, i, m.longReaderLines[i], want[i])
			}
		}
		if !strings.Contains(strings.Join(m.longReaderLines, "\n"), "\n\n") {
			t.Errorf("width %d: blank paragraph line was lost", width)
		}
	}
}

func TestLongReaderKeepsFooterAndComposerUsable(t *testing.T) {
	packet := interactionTestPacket("reader-actions", strings.Repeat("Some ordinary words to wrap nicely. ", 120))
	packet.Long = true
	m := newMessageInteractionTestModel(100, packet)
	m.openLongReader(packet)
	m.status = "Connected"
	m.connected = true
	m.user = packet.User
	var output bytes.Buffer
	m.client = &wireClient{enc: json.NewEncoder(&output)}

	// The reader is not a read-only overlay: the standard Send action remains
	// present, alongside the page-scroll actions and the normal textarea.
	_, targets := m.renderStatusBar(false, false)
	if !targets.hasSend || !targets.hasPageScroll {
		t.Fatalf("reader footer lost actions: %+v", targets)
	}
	m.input.SetValue("sent without closing reader")
	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft, X: targets.sendStart, Y: m.height-1,
	})
	m = updated.(model)
	if m.longReader == nil {
		t.Fatal("Send footer closed the reader")
	}
	if m.input.Value() != "" {
		t.Fatal("Send footer did not clear composer")
	}
	var sent Packet
	if err := json.NewDecoder(&output).Decode(&sent); err != nil {
		t.Fatal(err)
	}
	if sent.Type != "message" || sent.Text != "sent without closing reader" {
		t.Fatalf("Send footer transmitted incorrect packet: %#v", sent)
	}

	// Page controls scroll the reader, not the hidden chat history.
	updated, _ = m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft, X: targets.pgDnStart, Y: m.height-1,
	})
	m = updated.(model)
	if m.longReaderScroll == 0 {
		t.Fatal("PgDn footer did not scroll reader")
	}
	if m.scroll != 0 {
		t.Fatal("PgDn footer unexpectedly scrolled chat history")
	}

	// Enter uses the same shared send path, without closing the reader.
	output.Reset()
	m.input.SetValue("sent by Enter")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 13})
	m = updated.(model)
	if m.longReader == nil {
		t.Fatal("Enter closed the reader")
	}
	if err := json.NewDecoder(&output).Decode(&sent); err != nil {
		t.Fatal(err)
	}
	if sent.Text != "sent by Enter" {
		t.Fatalf("Enter transmitted incorrect packet: %#v", sent)
	}
}
