package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func whisperReplyTestPacket(id, sender, recipient, body string) Packet {
	return Packet{
		Type: "whisper_result", ID: id, Room: "test",
		User: User{Name: sender, Color: "#ff3399"},
		WhisperTo: recipient, Text: body,
		Time: time.Date(2026, time.January, 2, 15, 4, 0, 0, time.UTC),
	}
}

func TestWhisperHoverClickUsesExistingReplyComposer(t *testing.T) {
	packet := whisperReplyTestPacket("private-target", "Alice", "Postal", "secret message body")
	m := newMessageInteractionTestModel(100, packet)
	m.user = User{Name:"Postal"}
	m.resizeInput()
	m.input.SetValue("already drafted")
	bodies := m.messageBodyRanges(packet, m.width)
	if len(bodies) == 0 || bodies[0].End <= bodies[0].Start {
		t.Fatalf("whisper body hitbox is missing: %+v", bodies)
	}
	left := 0
	if !narrowBordersHidden(m.width) {left = 1}
	x, y := left+bodies[0].Start+1, 2
	if got := m.messageIDAt(x,y); got != packet.ID {
		t.Fatalf("whisper body hitbox returned %q instead of %q",got,packet.ID)
	}
	m.mousePositionKnown = true
	m.lastMouseX, m.lastMouseY = x, y
	m.refreshMessageHover()
	if m.hoveredMessageID != packet.ID {
		t.Fatalf("whisper failed to hover: got %q",m.hoveredMessageID)
	}
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X:x, Y:y})
	m = updated.(model)
	if m.replyTargetID != packet.ID || m.input.Value() != "already drafted" {
		t.Fatalf("whisper click did not use shared reply composer: id=%q draft=%q",m.replyTargetID,m.input.Value())
	}
	m.cancelReply()
	if m.replyTargetID != "" || m.input.Value() != "already drafted" {
		t.Fatal("cancel reply did not preserve whisper reply draft")
	}
}

func TestWhisperReplyRoutesPrivatelyForSenderAndRecipient(t *testing.T) {
	for _, tc := range []struct{
		name, currentUser, origin, target, want string
	}{
		{"incoming", "Postal", "Alice", "Postal", "Alice"},
		{"outgoing", "Alice", "Alice", "Postal", "Postal"},
	} {
		t.Run(tc.name,func(t *testing.T){
			parent := whisperReplyTestPacket("whisper-original",tc.origin,tc.target,"initial")
			m := newMessageInteractionTestModel(100,parent)
			m.user = User{Name:tc.currentUser}
			m.room = "test"
			var out bytes.Buffer
			m.client = &wireClient{enc:json.NewEncoder(&out)}
			m.beginReply(parent.ID)
			if m.replyTargetID != parent.ID {t.Fatal("reply selection failed")}
			if !m.sendMessage("private answer",m.replyTargetID) {
				t.Fatal("whisper reply was not sent")
			}
			var packet Packet
			if err := json.NewDecoder(&out).Decode(&packet);err != nil {t.Fatal(err)}
			if packet.Type!="whisper" || packet.Text!="private answer" ||
				packet.WhisperTo!="@"+tc.want || packet.ReplyTo!=parent.ID ||
				packet.Room!="test" {
				t.Fatalf("reply escaped private routing: %+v",packet)
			}
			// The same selected composer must not allow a public message
			// merely because its content resembles a normal slash command.
			out.Reset()
			if !m.sendMessage("/me secret action",m.replyTargetID) {t.Fatal("special draft failed")}
			if err:=json.NewDecoder(&out).Decode(&packet);err!=nil {t.Fatal(err)}
			if packet.Type!="whisper" || packet.Text!="/me secret action" {
				t.Fatalf("slash-like reply escaped into public chat: %+v",packet)
			}
		})
	}
}

func TestWhisperReplyUsesExistingNavigationMarkers(t *testing.T) {
	parent := whisperReplyTestPacket("private-parent","Alice","Postal","first secret")
	reply := whisperReplyTestPacket("private-reply","Postal","Alice","second secret")
	reply.ReplyTo = parent.ID
	m := newMessageInteractionTestModel(100,parent,reply)
	for _, packet := range []Packet{parent,reply} {
		if got,want:=m.renderPacketLineCount(packet,m.width),len(m.renderPacketLines(packet,m.width));got!=want {
			t.Fatalf("whisper row count %q = %d; renderer gave %d",packet.ID,got,want)
		}
	}
	parentLines:=strings.Join(m.renderPacketLines(parent,m.width),"\n")
	replyLines:=strings.Join(m.renderPacketLines(reply,m.width),"\n")
	if !strings.Contains(stripANSISequences(parentLines),"[1]") ||
		!strings.Contains(stripANSISequences(replyLines),"[Re]") {
		t.Fatalf("shared reply markers missing: parent=%q reply=%q",parentLines,replyLines)
	}
	parentLinks:=m.messageLinkRanges(parent,m.width)
	if len(parentLinks)==0 || len(parentLinks[0])==0 || parentLinks[0][0].TargetID!=reply.ID {
		t.Fatalf("numbered whisper reply link missing: %+v",parentLinks)
	}
	replyLinks:=m.messageLinkRanges(reply,m.width)
	if len(replyLinks)==0 || len(replyLinks[0])==0 || replyLinks[0][0].TargetID!=parent.ID {
		t.Fatalf("whisper [Re] jump target missing: %+v",replyLinks)
	}
	badge := replyLinks[0][0]
	if got:=m.messageLinkAt(badge.Start+1,3);got!=parent.ID {
		t.Fatalf("whisper [Re] click target=%q; want %q",got,parent.ID)
	}
}

func TestWhisperBodyHitboxesWorkAtCompactWidths(t *testing.T) {
	for _, width := range []int{20, 32, 48, 49, 80} {
		p := whisperReplyTestPacket("compact-whisper","Alice","Postal",
			"this whisper body must remain clickable even when it wraps across the whole screen")
		m := newMessageInteractionTestModel(width,p)
		ranges:=m.messageBodyRanges(p,width)
		if got,want:=len(ranges),len(m.renderPacketLines(p,width));got!=want {
			t.Fatalf("width=%d: %d hitboxes for %d rendered rows",width,got,want)
		}
		found:=false
		for row,hit:=range ranges {
			if hit.End <= hit.Start {continue}
			left:=0
			if !narrowBordersHidden(width) {left=1}
			x,y:=left+hit.Start,2+row
			if y>=2+max(1,m.messageHeight()-1) {continue}
			if id:=m.messageIDAt(x,y);id!=p.ID {
				t.Fatalf("width=%d row=%d: body click returned %q",width,row,id)
			}
			found=true
			break
		}
		if !found {
			t.Fatalf("width=%d: no visible whisper body is clickable",width)
		}
	}
}

func TestWhisperReplyMarkerRemainsClickableAfterIncomingAppend(t *testing.T) {
	// Both participants receive the same private packets but maintain
	// independent viewport caches. Reproduce the incremental arrival of
	// Daniel2's "bruh" reply on each client, rather than rebuilding history.
	for _, name := range []string{"Daniel1", "Daniel2"} {
		t.Run(name, func(t *testing.T) {
			parent := whisperReplyTestPacket("whisper-daniel1", "Daniel1", "Daniel2", "hey")
			reply := whisperReplyTestPacket("whisper-daniel2", "Daniel2", "Daniel1", "bruh")
			reply.ReplyTo = parent.ID
			m := newMessageInteractionTestModel(100, parent)
			m.user.Name = name
			// Prime the lazy interaction cache before the reply arrives.
			if link := m.messageLinkRanges(parent, m.width); len(link) == 0 {
				t.Fatal("initial private message did not have an interaction row")
			}
			m.appendMessage(reply)
			m.renderMessageLines()

			links := m.cachedLinkRangesForMessage(parent.ID, 0)
			var link messageLinkRange
			markerRow := -1
			for row, entries := range links {
				for _, entry := range entries {
					if entry.TargetID == reply.ID {
						link, markerRow = entry, row
						break
					}
				}
			}
			if markerRow == -1 {
				t.Fatalf("%s cannot find clickable [1] reply link", name)
			}
			absolute := m.messageRowsByID[parent.ID].Start + markerRow
			visible := stripANSISequences(m.renderMessageLines()[absolute])
			cells := []rune(visible)
			if link.End > len(cells) || string(cells[link.Start:link.End]) != "[1]" {
				t.Fatalf("%s reply link hitbox does not match displayed marker: row=%q hit=%+v", name, visible, link)
			}
			left := 1
			x, y := left+link.Start+1, 2+markerRow
			if target := m.messageLinkAt(x, y); target != reply.ID {
				t.Fatalf("%s clicked [1] but got %q instead of %q", name, target, reply.ID)
			}
			updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
			m = updated.(model)
			if m.highlightedMessageID != reply.ID {
				t.Fatalf("%s clicking [1] did not navigate to the reply: selected %q", name, m.highlightedMessageID)
			}
		})
	}
}

func TestIncomingWhisperReplyUpdatesParentMarkersIncrementally(t *testing.T) {
	parent := whisperReplyTestPacket("private-live-parent","Alice","Postal","initial whisper")
	reply := whisperReplyTestPacket("private-live-reply","Postal","Alice","private response")
	reply.ReplyTo = parent.ID
	m := newMessageInteractionTestModel(100,parent)
	originalRows := m.messageRowsByID[parent.ID]
	m.appendMessage(reply)
	if got:=m.messageByID[reply.ID].ID; got!=reply.ID {
		t.Fatalf("new whisper reply was not indexed: %q",got)
	}
	replies:=m.repliesFor(parent.ID)
	if len(replies)!=1 || replies[0]!=reply.ID {
		t.Fatalf("private reply not connected to parent: %v",replies)
	}
	if got:=m.messageRowsByID[parent.ID];got.Start!=originalRows.Start {
		t.Fatalf("parent whisper was displaced: %+v",got)
	}
	if !strings.Contains(stripANSISequences(strings.Join(m.renderPacketLines(parent,m.width),"\n")),"[1]") {
		t.Fatal("new private reply did not add the shared numbered reply marker")
	}
}
