package main

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

func newMessageInteractionTestModel(width int, messages ...Packet) model {
	cfg := defaultConfig()
	input := textarea.New()
	input.SetHeight(2)
	m := model{
		cfg:      cfg,
		width:    width,
		height:   24,
		input:    input,
		messages: append([]Packet(nil), messages...),
		styles:   makeStyles(cfg),
	}
	m.rebuildMessageCache()
	return m
}

func interactionTestPacket(id, text string) Packet {
	return Packet{
		Type: "message",
		ID:   id,
		User: User{Name: "Alice", Color: "#7CFFCB"},
		Text: text,
		Time: time.Date(2026, time.January, 2, 15, 4, 0, 0, time.UTC),
	}
}

func TestMessageBodyHitboxExcludesUsernameAndTimestamp(t *testing.T) {
	packet := interactionTestPacket("message-1", "hello")
	m := newMessageInteractionTestModel(80, packet)

	ranges := m.messageBodyRanges(packet, m.width)
	if len(ranges) == 0 || ranges[0].End <= ranges[0].Start {
		t.Fatalf("expected a body hitbox, got %#v", ranges)
	}

	left := 1 // visible left border
	timestampWidth := lipgloss.Width(m.styles.time.Render("[15:04]"))
	usernameStart := timestampWidth + 1
	bodyStart := ranges[0].Start

	if got := m.messageIDAt(left+bodyStart, 2); got != packet.ID {
		t.Fatalf("body hitbox returned %q, want %q", got, packet.ID)
	}
	if got := m.messageIDAt(left+usernameStart, 2); got != "" {
		t.Fatalf("username hitbox returned %q, want no message", got)
	}
	if got := m.messageIDAt(left, 2); got != "" {
		t.Fatalf("timestamp hitbox returned %q, want no message", got)
	}
	if got := m.messageIDAt(left+ranges[0].End, 2); got != "" {
		t.Fatalf("space after body returned %q, want no message", got)
	}
}

func TestCompactMessageBodyHitboxesFollowWrappedRows(t *testing.T) {
	packet := interactionTestPacket("compact-1", strings.Repeat("word ", 24))
	m := newMessageInteractionTestModel(48, packet)

	ranges := m.messageBodyRanges(packet, m.width)
	if len(ranges) < 3 {
		t.Fatalf("expected multiple compact rows, got %d", len(ranges))
	}
	if ranges[0].End <= ranges[0].Start || ranges[1].End <= ranges[1].Start || ranges[2].End <= ranges[2].Start {
		t.Fatalf("expected body hitboxes on the first three rows, got %#v", ranges)
	}

	for row := 0; row < 3; row++ {
		x := ranges[row].Start
		if got := m.messageIDAt(x, 2+row); got != packet.ID {
			t.Fatalf("row %d body hitbox returned %q, want %q", row, got, packet.ID)
		}
	}
	if got := m.messageIDAt(0, 2); got != "" {
		t.Fatalf("compact username hitbox returned %q, want no message", got)
	}
	if got := m.messageIDAt(0, 3); got != "" {
		t.Fatalf("compact timestamp hitbox returned %q, want no message", got)
	}
}

func TestReplyMarkersNavigateInArrivalOrder(t *testing.T) {
	parent := interactionTestPacket("parent", "original message")
	first := interactionTestPacket("reply-1", "first reply")
	first.ReplyTo = parent.ID
	second := interactionTestPacket("reply-2", "second reply")
	second.ReplyTo = parent.ID

	m := newMessageInteractionTestModel(80, parent, first, second)

	parentLinks := m.messageLinkRanges(parent, m.width)
	if len(parentLinks) == 0 {
		t.Fatal("expected a caret marker row on the original message")
	}
	markers := parentLinks[len(parentLinks)-1]
	if len(markers) != 2 {
		t.Fatalf("got %d caret links, want 2", len(markers))
	}
	if markers[0].Start != 0 || markers[0].TargetID != first.ID {
		t.Fatalf("first caret = %#v, want first reply", markers[0])
	}
	if markers[1].Start != 2 || markers[1].TargetID != second.ID {
		t.Fatalf("second caret = %#v, want second reply", markers[1])
	}

	replyLinks := m.messageLinkRanges(first, m.width)
	if len(replyLinks) == 0 || len(replyLinks[0]) == 0 || replyLinks[0][0].TargetID != parent.ID {
		t.Fatalf("[Replied] link does not target the original message: %#v", replyLinks)
	}

	lateJoin := newMessageInteractionTestModel(80, first)
	lateJoinLinks := lateJoin.messageLinkRanges(first, lateJoin.width)
	if len(lateJoinLinks) == 0 || len(lateJoinLinks[0]) != 0 {
		t.Fatalf("missing original message should not produce an active link: %#v", lateJoinLinks)
	}
}
