package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestReadPacketBatchesHistoryUntilEndMarker(t *testing.T) {
	var wire strings.Builder
	encoder := json.NewEncoder(&wire)
	for i := 0; i < historyBatchSize+2; i++ {
		if err := encoder.Encode(Packet{Type: "message", ID: strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := encoder.Encode(Packet{Type: "history_end"}); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Encode(Packet{Type: "online", Text: "4"}); err != nil {
		t.Fatal(err)
	}

	client := &wireClient{
		decoder:       json.NewDecoder(strings.NewReader(wire.String())),
		historyReplay: true,
	}

	firstMessage := readPacket(client)()
	first, ok := firstMessage.(historyBatchReceivedMsg)
	if !ok {
		t.Fatalf("first read returned %T, want historyBatchReceivedMsg", firstMessage)
	}
	if len(first.Packets) != historyBatchSize {
		t.Fatalf("first history chunk contains %d packets, want %d", len(first.Packets), historyBatchSize)
	}
	if first.Complete {
		t.Fatal("first history chunk unexpectedly marked replay complete")
	}
	if !client.historyReplay {
		t.Fatal("history replay ended before the history_end marker")
	}

	secondMessage := readPacket(client)()
	second, ok := secondMessage.(historyBatchReceivedMsg)
	if !ok {
		t.Fatalf("second read returned %T, want historyBatchReceivedMsg", secondMessage)
	}
	if len(second.Packets) != 2 {
		t.Fatalf("second history chunk contains %d packets, want 2", len(second.Packets))
	}
	if !second.Complete {
		t.Fatal("history_end marker did not complete replay")
	}
	if client.historyReplay {
		t.Fatal("history replay flag remained enabled after history_end")
	}

	nextMessage := readPacket(client)()
	next, ok := nextMessage.(messageReceivedMsg)
	if !ok {
		t.Fatalf("post-history read returned %T, want messageReceivedMsg", nextMessage)
	}
	if packet := Packet(next); packet.Type != "online" {
		t.Fatalf("post-history packet type = %q, want online", packet.Type)
	}
}

func TestAppendMessagesKeepsCachedReplyRowsConsistent(t *testing.T) {
	parent := interactionTestPacket("batch-parent", "An original message")
	firstReply := interactionTestPacket("batch-reply-1", "First reply")
	firstReply.ReplyTo = parent.ID
	secondReply := interactionTestPacket("batch-reply-2", "Second reply")
	secondReply.ReplyTo = parent.ID

	m := newMessageInteractionTestModel(80, parent)
	m.appendMessages([]Packet{firstReply, secondReply})

	if got := len(m.replyIDsByMessageID[parent.ID]); got != 2 {
		t.Fatalf("parent reply index contains %d replies, want 2", got)
	}

	cachedLines := append([]string(nil), m.renderedLines...)
	m.rebuildMessageCache()
	if got, want := strings.Join(cachedLines, "\n"), strings.Join(m.renderedLines, "\n"); got != want {
		t.Fatal("incrementally updated message cache differs from a full rebuild")
	}
}

func TestAppendInitialHistoryBatchBuildsConsistentReplyIndexes(t *testing.T) {
	parent := interactionTestPacket("initial-parent", "A message from history")
	firstReply := interactionTestPacket("initial-reply-1", "First historical reply")
	firstReply.ReplyTo = parent.ID
	secondReply := interactionTestPacket("initial-reply-2", "Second historical reply")
	secondReply.ReplyTo = parent.ID

	m := newMessageInteractionTestModel(80)
	m.appendMessages([]Packet{parent, firstReply, secondReply})

	if got := len(m.replyIDsByMessageID[parent.ID]); got != 2 {
		t.Fatalf("parent reply index contains %d replies, want 2", got)
	}
	cachedLines := append([]string(nil), m.renderedLines...)
	m.rebuildMessageCache()
	if got, want := strings.Join(cachedLines, "\n"), strings.Join(m.renderedLines, "\n"); got != want {
		t.Fatal("initial history batch cache differs from a full rebuild")
	}
}

