package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func newRenderCacheTestModel() model {
	cfg := defaultConfig()
	cfg.Name = "tester"
	cfg.UserColor = "#FFFFFF"
	cfg.Theme.Background = "#000000"
	cfg.Theme.TextColor = "#FFFFFF"
	cfg.Theme.AccentColor = "#00AEEF"
	cfg.InputHeight = 1

	m := newModel(cfg, "test", false, nil)
	m.width = 80
	m.height = 18
	m.input.SetWidth(74)
	m.input.SetHeight(1)
	m.messages = make([]Packet, 20)
	for i := range m.messages {
		m.messages[i] = Packet{
			Type: "message",
			ID:   fmt.Sprintf("message-%02d", i),
			Time: time.Date(2026, time.January, 1, 12, i, 0, 0, time.Local),
			User: User{Name: "tester", Color: "#FFFFFF"},
			Text: fmt.Sprintf("body-%02d", i),
		}
	}
	return m
}

func cachedMessageText(m model, index int) string {
	rows := m.messageRowRanges[index]
	return strings.Join(m.renderedLines[rows.Start:rows.End], "\n")
}

func TestRefreshRenderedWindowReusesOverlappingRows(t *testing.T) {
	m := newRenderCacheTestModel()
	m.rebuildMessageCache()

	oldStart := m.renderWindowStart
	if oldStart < 1 {
		t.Fatalf("expected a non-empty scroll buffer before the first message, got start %d", oldStart)
	}
	newlyVisibleIndex := oldStart - 1
	overlappingIndex := oldStart

	// Same-width edits make rerendering observable without changing row counts.
	newlyVisibleText := fmt.Sprintf("edit-%02d", newlyVisibleIndex)
	overlappingText := fmt.Sprintf("edit-%02d", overlappingIndex)
	m.messages[newlyVisibleIndex].Text = newlyVisibleText
	m.messages[overlappingIndex].Text = overlappingText

	m.scroll = 1
	m.refreshRenderedWindow()

	if m.renderWindowStart != newlyVisibleIndex {
		t.Fatalf("render window start = %d, want %d", m.renderWindowStart, newlyVisibleIndex)
	}
	if got := cachedMessageText(m, newlyVisibleIndex); !strings.Contains(got, newlyVisibleText) {
		t.Fatalf("newly exposed message was not rendered: %q", got)
	}
	if got := cachedMessageText(m, overlappingIndex); !strings.Contains(got, fmt.Sprintf("body-%02d", overlappingIndex)) {
		t.Fatalf("overlapping message's cached row was not retained: %q", got)
	} else if strings.Contains(got, overlappingText) {
		t.Fatalf("overlapping message was rerendered instead of reusing its cached row: %q", got)
	}
}

func TestRefreshRenderedWindowClearsRowsOutsideNewWindow(t *testing.T) {
	m := newRenderCacheTestModel()
	m.rebuildMessageCache()

	oldEnd := m.renderWindowEnd
	if oldEnd <= 0 {
		t.Fatal("expected an initially populated render window")
	}

	// Jump to the beginning of history so the old window's trailing rows leave
	// the cache window completely.
	m.scroll = m.maxScroll()
	m.refreshRenderedWindow()

	if m.renderWindowEnd >= oldEnd {
		t.Fatalf("expected the new window to end before old end %d, got %d", oldEnd, m.renderWindowEnd)
	}
	for i, line := range m.renderedLines[m.renderWindowEnd:oldEnd] {
		if line != "" {
			t.Fatalf("stale rendered row at absolute row %d was not cleared", m.renderWindowEnd+i)
		}
	}
}
