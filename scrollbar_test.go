package main

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func makeScrollbarTestMessages(count int) []Packet {
	messages := make([]Packet, count)
	for i := range messages {
		messages[i] = interactionTestPacket(fmt.Sprintf("scrollbar-%d", i), "short line")
	}
	return messages
}

func TestChatScrollbarOneCellThumbRoundTripsEveryTrackPosition(t *testing.T) {
	m := newMessageInteractionTestModel(80, makeScrollbarTestMessages(2000)...)
	total := len(m.renderMessageLines())
	_, thumbHeight, trackHeight := m.scrollbarThumbForLineCount(total)
	if thumbHeight != 1 {
		t.Fatalf("thumb height = %d, want one cell (track height %d, total rows %d)", thumbHeight, trackHeight, total)
	}
	maxTop := trackHeight - thumbHeight
	if maxTop < 1 {
		t.Fatalf("test scrollbar has no travel range: track=%d thumb=%d", trackHeight, thumbHeight)
	}

	previousScroll := m.maxScroll()
	for targetTop := 0; targetTop <= maxTop; targetTop++ {
		m.setScrollFromThumbTopForLineCount(targetTop, total)
		gotTop, gotHeight, _ := m.scrollbarThumbForLineCount(total)
		if gotTop != targetTop {
			t.Fatalf("thumb position after dragging to %d = %d (height %d), want exact position %d; scroll=%d",
				targetTop, gotTop, gotHeight, targetTop, m.scroll)
		}
		if m.scroll > previousScroll {
			t.Fatalf("scroll increased while dragging thumb downward: previous=%d current=%d at track position %d",
				previousScroll, m.scroll, targetTop)
		}
		previousScroll = m.scroll
	}

	if m.scroll != 0 {
		t.Fatalf("bottom thumb position left scroll=%d, want 0", m.scroll)
	}
	m.setScrollFromThumbTopForLineCount(0, total)
	if m.scroll != m.maxScroll() {
		t.Fatalf("top thumb position set scroll=%d, want maximum %d", m.scroll, m.maxScroll())
	}
}

func TestChatScrollbarTrackClickPlacesOneCellThumbAtClickedRow(t *testing.T) {
	m := newMessageInteractionTestModel(80, makeScrollbarTestMessages(2000)...)
	total := len(m.renderMessageLines())
	_, thumbHeight, trackHeight := m.scrollbarThumbForLineCount(total)
	if thumbHeight != 1 {
		t.Fatalf("thumb height = %d, want one cell", thumbHeight)
	}

	for trackPosition := 0; trackPosition < trackHeight; trackPosition++ {
		m.setScrollFromTrack(trackPosition)
		gotTop, gotHeight, _ := m.scrollbarThumbForLineCount(total)
		if gotTop != trackPosition {
			t.Fatalf("clicking track row %d moved chat to scroll=%d but thumb rendered at %d (height %d)",
				trackPosition, m.scroll, gotTop, gotHeight)
		}
	}
}

func TestPointerBelowChatScrollbarDoesNotScrollWhenNotDragging(t *testing.T) {
	m := newMessageInteractionTestModel(80, makeScrollbarTestMessages(100)...)
	m.scroll = m.maxScroll() / 2
	before := m.scroll

	// The row below the bottom arrow is outside the chat scrollbar hit area.
	belowScrollbarY := 2 + m.messageHeight()
	updated, cmd := m.Update(tea.MouseMotionMsg{
		X: m.chatScrollbarX(),
		Y: belowScrollbarY,
	})
	if cmd == nil {
		t.Fatal("mouse motion did not schedule its coalesced frame")
	}
	m = updated.(model)
	generation := m.mouseMotionFrameGeneration
	updated, _ = m.Update(mouseMotionFrameMsg{generation: generation})
	got := updated.(model)
	if got.scroll != before {
		t.Fatalf("hovering below scrollbar changed scroll from %d to %d", before, got.scroll)
	}

	updated, _ = got.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      got.chatScrollbarX(),
		Y:      belowScrollbarY,
	})
	if after := updated.(model).scroll; after != before {
		t.Fatalf("clicking below scrollbar changed scroll from %d to %d", before, after)
	}
}

func TestUsersScrollbarOneCellThumbRoundTripsEveryTrackPosition(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.usersOpen = true
	m.onlineUsers = make([]User, 2000)
	for i := range m.onlineUsers {
		m.onlineUsers[i] = User{Name: fmt.Sprintf("user-%04d", i), Color: "#FFFFFF"}
	}
	total := m.userRowCount()
	_, thumbHeight, trackHeight := m.usersScrollbarThumbForRowCount(total)
	if thumbHeight != 1 {
		t.Fatalf("users thumb height = %d, want one cell", thumbHeight)
	}
	maxTop := trackHeight - thumbHeight
	for targetTop := 0; targetTop <= maxTop; targetTop++ {
		m.setUsersScrollFromThumbTop(targetTop)
		gotTop, _, _ := m.usersScrollbarThumbForRowCount(total)
		if gotTop != targetTop {
			t.Fatalf("users thumb after dragging to %d rendered at %d", targetTop, gotTop)
		}
	}
}
