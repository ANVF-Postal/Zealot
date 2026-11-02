package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func TestReplyComposerKeepsDraftAndUsesSeparatePrefix(t *testing.T) {
	packet := interactionTestPacket("reply-target", "original")
	m := newMessageInteractionTestModel(80, packet)
	m.resizeInput()
	normalWidth := m.input.Width()
	m.input.SetValue("draft text")

	m.beginReply(packet.ID)
	if m.replyTargetID != packet.ID {
		t.Fatalf("reply target = %q, want %q", m.replyTargetID, packet.ID)
	}
	if got := m.input.Value(); got != "draft text" {
		t.Fatalf("reply mode changed draft to %q", got)
	}
	if got, want := m.input.Width(), normalWidth-len([]rune(replyComposerPrefix)); got != want {
		t.Fatalf("reply textarea width = %d, want %d", got, want)
	}

	m.cancelReply()
	if m.replyTargetID != "" {
		t.Fatalf("reply target after cancellation = %q, want empty", m.replyTargetID)
	}
	if got := m.input.Value(); got != "draft text" {
		t.Fatalf("cancelling reply changed draft to %q", got)
	}
	if got := m.input.Width(); got != normalWidth {
		t.Fatalf("textarea width after cancellation = %d, want %d", got, normalWidth)
	}
}

func TestAppViewEnablesAllMouseMotion(t *testing.T) {
	if got := appView("").MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("appView MouseMode = %v, want MouseModeAllMotion", got)
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

func TestWideWrappedMessageHitboxExcludesContinuationIndent(t *testing.T) {
	packet := interactionTestPacket("wide-wrap", strings.Repeat("word ", 32))
	m := newMessageInteractionTestModel(80, packet)

	ranges := m.messageBodyRanges(packet, m.width)
	if len(ranges) < 2 || ranges[1].End <= ranges[1].Start {
		t.Fatalf("expected a wrapped body row, got %#v", ranges)
	}
	if ranges[1].Start == 0 {
		t.Fatalf("expected continuation indentation before body text, got %#v", ranges[1])
	}

	left := 1
	if got := m.messageIDAt(left+ranges[1].Start-1, 3); got != "" {
		t.Fatalf("continuation indentation returned %q, want no message", got)
	}
	if got := m.messageIDAt(left+ranges[1].Start, 3); got != packet.ID {
		t.Fatalf("wrapped body hitbox returned %q, want %q", got, packet.ID)
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

	parentLines := m.renderPacketLines(parent, m.width)
	if len(parentLines) == 0 {
		t.Fatal("expected the original message to render")
	}
	if !strings.Contains(parentLines[0], "original message") ||
		!strings.Contains(parentLines[0], "[1]") ||
		!strings.Contains(parentLines[0], "[2]") {
		t.Fatalf("expected both numbered markers inline after the body; first row = %q", parentLines[0])
	}
	if got, want := lipgloss.Width(parentLines[0]), chatContentWidth(m.width); got != want {
		t.Fatalf("inline marker row width = %d, want content width %d (marker must not enter scrollbar column)", got, want)
	}

	parentLinks := m.messageLinkRanges(parent, m.width)
	if len(parentLinks) == 0 { t.Fatal("expected inline reply marker links") }
	markers := parentLinks[0]
	if len(markers) != 2 { t.Fatalf("got %d inline marker links, want 2", len(markers)) }
	if markers[0].TargetID != first.ID || markers[1].TargetID != second.ID { t.Fatalf("inline marker order = %#v, want first then second reply", markers) }
	if markers[0].Start <= 0 { t.Fatalf("markers should follow message text, got %#v", markers) }
	if got := m.messageLinkAt(markers[0].Start+1, 2); got != first.ID { t.Fatalf("first marker click returned %q, want %q", got, first.ID) }
	if got := m.messageLinkAt(markers[1].Start+1, 2); got != second.ID { t.Fatalf("second marker click returned %q, want %q", got, second.ID) }

	replyLinks := m.messageLinkRanges(first, m.width)
	if len(replyLinks) == 0 || len(replyLinks[0]) == 0 || replyLinks[0][0].TargetID != parent.ID {
		t.Fatalf("[Replied] link does not target the original message: %#v", replyLinks)
	}
	badge := replyLinks[0][0]
	if got := m.messageLinkAt(badge.Start+1, 3); got != parent.ID {
		t.Fatalf("[Replied] click returned %q, want %q", got, parent.ID)
	}
	replyBody := m.messageBodyRanges(first, m.width)
	if len(replyBody) == 0 || replyBody[0].End <= replyBody[0].Start {
		t.Fatalf("expected a reply body hitbox, got %#v", replyBody)
	}
	if got := m.messageIDAt(badge.Start+1, 3); got != "" {
		t.Fatalf("[Replied] label was treated as body text for %q", got)
	}
	if got := m.messageIDAt(1+replyBody[0].Start, 3); got != first.ID {
		t.Fatalf("reply body click returned %q, want %q", got, first.ID)
	}

	lateJoin := newMessageInteractionTestModel(80, first)
	lateJoinLinks := lateJoin.messageLinkRanges(first, lateJoin.width)
	if len(lateJoinLinks) == 0 || len(lateJoinLinks[0]) != 0 {
		t.Fatalf("missing original message should not produce an active link: %#v", lateJoinLinks)
	}
}
