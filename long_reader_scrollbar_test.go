package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Check the long reader against the exact geometry shared with main chat,
// including the one-cell thumb case that previously needed careful rounding.
func longReaderScrollbarTestModel() model {
	packet := interactionTestPacket("reader-scrollbar", strings.Repeat("a test word with spaces to wrap reliably. ", 4000))
	packet.Long = true
	m := newMessageInteractionTestModel(80, packet)
	m.openLongReader(packet)
	return m
}

func TestLongReaderScrollbarUsesSharedChatGeometry(t *testing.T) {
	m := longReaderScrollbarTestModel()
	top, thumb, track := m.longReaderScrollbarThumb()
	wantTop, wantThumb, wantTrack := scrollbarGeometry(
		len(m.longReaderLines), m.longReaderVisibleHeight(),
		m.messageHeight()-2, m.longReaderScroll,
	)
	if top != wantTop || thumb != wantThumb || track != wantTrack {
		t.Fatalf("reader geometry (%d,%d,%d) differs from shared chat geometry (%d,%d,%d)", top, thumb, track, wantTop, wantThumb, wantTrack)
	}
	if thumb != 1 {
		t.Fatalf("expected one-cell thumb for large reader, got %d", thumb)
	}
	if !m.longReaderScrollbarContains(m.chatScrollbarX(), 2) ||
		!m.longReaderScrollbarContains(m.chatScrollbarX(), 1+m.messageHeight()) ||
		m.longReaderScrollbarContains(m.chatScrollbarX()-1, 3) {
		t.Fatal("reader scrollbar hitbox is not aligned with main chat")
	}
}

func TestLongReaderScrollbarTrackClickAndDrag(t *testing.T) {
	m := longReaderScrollbarTestModel()
	_, thumb, track := m.longReaderScrollbarThumb()
	if thumb != 1 || track <= 2 {
		t.Fatalf("unexpected test scrollbar geometry: thumb=%d track=%d", thumb, track)
	}
	for position := 0; position < track; position++ {
		m.longReaderDragging = false
		updated, _ := m.Update(tea.MouseClickMsg{
			Button: tea.MouseLeft, X: m.chatScrollbarX(), Y: 3+position,
		})
		m = updated.(model)
		got, _, _ := m.longReaderScrollbarThumb()
		if got != position {
			t.Fatalf("track click at %d yielded thumb=%d", position, got)
		}
	}
	m.setLongReaderScrollFromThumbTop(0)
	m.longReaderDragging = true
	m.longReaderDragOffset = 0
	for position := 0; position < track; position++ {
		m.applyMouseMotion(m.chatScrollbarX(), 3+position)
		got, _, _ := m.longReaderScrollbarThumb()
		if got != position {
			t.Fatalf("drag to %d yielded thumb=%d", position, got)
		}
	}
	if m.longReaderScroll != m.maxLongReaderScroll() {
		t.Fatalf("bottom thumb did not reveal end: scroll=%d max=%d", m.longReaderScroll, m.maxLongReaderScroll())
	}
	m.applyMouseMotion(m.chatScrollbarX(), -100)
	if m.longReaderScroll != 0 {
		t.Fatal("drag outside top did not clamp")
	}
	m.applyMouseMotion(m.chatScrollbarX(), 10000)
	if m.longReaderScroll != m.maxLongReaderScroll() {
		t.Fatal("drag outside bottom did not clamp")
	}
}

func TestLongReaderScrollbarArrowClickHoldAndRelease(t *testing.T) {
	m := longReaderScrollbarTestModel()
	m.longReaderScroll = 50
	updated, cmd := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft, X: m.chatScrollbarX(), Y: 2,
	})
	m = updated.(model)
	if m.longReaderScroll != 47 || cmd == nil || m.longReaderScrollbarHoldDirection != -1 {
		t.Fatalf("up arrow did not step/hold: scroll=%d hold=%d", m.longReaderScroll, m.longReaderScrollbarHoldDirection)
	}
	generation := m.longReaderScrollbarHoldGeneration
	updated, cmd = m.Update(longReaderScrollbarHoldMsg{generation: generation})
	m = updated.(model)
	if m.longReaderScroll != 44 || cmd == nil {
		t.Fatalf("held up arrow did not repeat: scroll=%d", m.longReaderScroll)
	}
	updated, _ = m.Update(tea.MouseReleaseMsg{})
	m = updated.(model)
	if m.longReaderScrollbarHoldDirection != 0 || m.longReaderDragging {
		t.Fatal("release did not cancel reader scrolling")
	}
	updated, _ = m.Update(longReaderScrollbarHoldMsg{generation: generation})
	if updated.(model).longReaderScroll != m.longReaderScroll {
		t.Fatal("stale held-arrow timer moved reader after release")
	}
	m.longReaderScroll = 50
	updated, _ = m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft, X: m.chatScrollbarX(), Y: 1+m.messageHeight(),
	})
	m = updated.(model)
	if m.longReaderScroll != 53 || m.longReaderScrollbarHoldDirection != 1 {
		t.Fatalf("down arrow did not scroll downward: %d", m.longReaderScroll)
	}
	m.closeLongReader()
	if m.longReaderDragging || m.longReaderScrollbarHoldDirection != 0 {
		t.Fatal("closing reader left scrollbar interaction active")
	}
}

func TestLongReaderScrollbarThumbRenderedAtTrackClickPosition(t *testing.T) {
	m := longReaderScrollbarTestModel()
	_, thumb, track := m.longReaderScrollbarThumb()
	if thumb != 1 {
		t.Fatalf("expected one-cell thumb, got %d", thumb)
	}
	for _, position := range []int{0, 1, track / 2, track - 1} {
		m.setLongReaderScrollFromThumbTop(position)
		rows := strings.Split(stripANSISequences(m.renderLongReaderPanel()), "\n")
		// The chat border is row zero, the header row is row one,
		// and the scrollbar track starts at the blank spacer row (row two).
		trackRow := 2 + position
		if trackRow >= len(rows) || !strings.Contains(rows[trackRow], "█") {
			t.Fatalf("thumb at %d is not painted on its clickable track row %d: %q", position, trackRow, rows)
		}
	}
}
