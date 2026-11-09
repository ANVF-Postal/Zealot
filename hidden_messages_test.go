package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestHiddenMessageDisplayAndBounds(t *testing.T) {
	long := interactionTestPacket("long-1", strings.Repeat("long text ", 1000))
	long.Long = true
	spoiler := interactionTestPacket("spoiler-1", "The cake is a lie")
	spoiler.Spoiler = true
	for _, width := range []int{35, 80} {
		m := newMessageInteractionTestModel(width, long, spoiler)
		lines := m.renderPacketLines(long, width)
		plain := stripANSISequences(strings.Join(lines, "\n"))
		if !strings.Contains(plain, "[Long Message]") || strings.Contains(plain, "long text") {
			t.Errorf("width %d: long body leaked into chat: %q", width, plain)
		}
		if got := m.renderPacketLineCount(long, width); got != len(lines) {
			t.Errorf("width %d: long row count = %d, rendered = %d", width, got, len(lines))
		}
		spoilerLines := m.renderPacketLines(spoiler, width)
		hidden := stripANSISequences(strings.Join(spoilerLines, "\n"))
		if strings.Contains(hidden, "cake") || !strings.Contains(hidden, "####") {
			t.Errorf("width %d: spoiler was not masked: %q", width, hidden)
		}
		if got := m.renderPacketLineCount(spoiler, width); got != len(spoilerLines) {
			t.Errorf("width %d: spoiler count = %d, rendered = %d", width, got, len(spoilerLines))
		}
	}
}

func TestSpoilerClickRevealsOnlyLocallyAndEnablesReply(t *testing.T) {
	packet := interactionTestPacket("spoiler-2", "secret message")
	packet.Spoiler = true
	m := newMessageInteractionTestModel(80, packet)
	m.beginReply(packet.ID)
	if m.replyTargetID != "" {
		t.Fatal("unrevealed spoiler accepted a reply")
	}
	hitbox := m.messageBodyRanges(packet, m.width)[0]
	if hitbox.End <= hitbox.Start {
		t.Fatal("spoiler body has no clickable hitbox")
	}
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: 1+hitbox.Start, Y: 2}
	if got := m.messageIDAt(click.X, click.Y); got != packet.ID {
		t.Fatalf("hitbox returned %q", got)
	}
	updated, _ := m.Update(click)
	m = updated.(model)
	if !m.revealedSpoilers[packet.ID] {
		t.Fatal("click did not reveal spoiler")
	}
	if m.replyTargetID != "" {
		t.Fatal("revealing spoiler incorrectly started a reply")
	}
	if text := stripANSISequences(strings.Join(m.renderPacketLines(packet, m.width), "\n")); !strings.Contains(text, packet.Text) {
		t.Fatalf("spoiler remained concealed: %q", text)
	}
	m.beginReply(packet.ID)
	if m.replyTargetID != packet.ID {
		t.Fatal("revealed spoiler did not permit a reply")
	}
	other := newMessageInteractionTestModel(80, packet)
	if other.revealedSpoilers[packet.ID] {
		t.Fatal("spoiler reveal leaked into another client")
	}
}

func TestLongReaderClickScrollAndClose(t *testing.T) {
	packet := interactionTestPacket("long-2", strings.Repeat("A long sentence is here.\n", 200))
	packet.Long = true
	m := newMessageInteractionTestModel(80, packet)
	ranges := m.messageBodyRanges(packet, m.width)
	if len(ranges) == 0 || ranges[0].End <= ranges[0].Start {
		t.Fatal("long link has no hitbox")
	}
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: 1+ranges[0].Start, Y: 2}
	updated, _ := m.Update(click)
	m = updated.(model)
	if m.longReader == nil || m.longReader.ID != packet.ID {
		t.Fatal("click did not open long reader")
	}
	if len(m.longReaderLines) <= m.longReaderVisibleHeight() {
		t.Fatal("test message is not longer than reader viewport")
	}
	if !strings.Contains(stripANSISequences(m.renderLongReaderPanel()), "LONG MESSAGE") {
		t.Fatal("reader did not render its heading")
	}
	updated, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = updated.(model)
	if m.longReaderScroll != 3 {
		t.Fatalf("wheel did not scroll reader: %d", m.longReaderScroll)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 27})
	m = updated.(model)
	if m.longReader != nil {
		t.Fatal("Escape did not return to chat")
	}
	if got := m.renderPacketLineCount(packet, m.width); got > 4 {
		t.Fatalf("long reader inflated chat after closing: %d rows", got)
	}
}

func TestTerminalControlsCannotEscapeLongReaderOrRevealedSpoiler(t *testing.T) {
	raw := "hello\x1b]52;c;Zm9v\x07 world\x1b[2J"
	cleaned := terminalSafeText(raw)
	if strings.ContainsRune(cleaned, '\x1b') || strings.ContainsRune(cleaned, '\x07') {
		t.Fatalf("reader retained terminal controls: %q", cleaned)
	}
	packet := interactionTestPacket("spoiler-control", raw)
	packet.Spoiler = true
	m := newMessageInteractionTestModel(80, packet)
	m.revealSpoiler(packet.ID)
	rendered := strings.Join(m.renderPacketLines(packet, m.width), "\n")
	if strings.Contains(rendered, "\x1b]52;") || strings.Contains(rendered, "\x1b[2J") {
		t.Fatal("untrusted terminal control reached revealed spoiler")
	}
}

func TestSpoilerMaskPreservesSpacesAndMutedThemeStyling(t *testing.T) {
	packet := interactionTestPacket("spoiler-muted", "two words")
	packet.Spoiler = true
	m := newMessageInteractionTestModel(80, packet)
	if got := spoilerMask(packet.Text); got != "### #####" {
		t.Errorf("mask = %q", got)
	}
	row := m.renderPacketLines(packet, m.width)[0]
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.MutedTextColor)).Background(m.messageBackground(packet))
	if !strings.Contains(row, muted.Render("### #####")) {
		t.Errorf("hidden spoiler lacks muted colour: %q", row)
	}
	m.revealedSpoilers = map[string]bool{packet.ID: true}
	if got := m.displayedMessage(packet).Text; got != packet.Text {
		t.Fatalf("revealed text %q differs from original", got)
	}
}

func TestLongMessageInputLimitsAndCommandBoundary(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.input.CharLimit = maxMessage
	m.input.SetValue("/long ")
	m.input.CharLimit = maxLongMessage + len("/long ")
	if m.input.CharLimit <= maxMessage {
		t.Fatal("long composer limit not raised")
	}
	if maxLongMessage <= maxMessage {
		t.Fatal("long message cap must exceed normal chat cap")
	}
	if got := m.displayedMessage(Packet{
		Type: "message", ID: "long-x", Long: true, Text: strings.Repeat("a", maxMessage+1),
		Time: time.Now().UTC(),
	}).Text; got != "[Long Message]" {
		t.Fatalf("long text visible in chat: %q", got)
	}
}

func TestLongReaderHeaderTimestampAndSpacer(t *testing.T) {
	packet := interactionTestPacket("long-header", "First line\nSecond line")
	packet.Long = true
	packet.User.Name = "Daniel2"
	m := newMessageInteractionTestModel(80, packet)
	m.openLongReader(packet)

	rendered := stripANSISequences(m.renderLongReaderPanel())
	rows := strings.Split(rendered, "\n")
	wantHeader := "[LONG MESSAGE] [" + packet.Time.Local().Format("15:04") + "] Daniel2 · Esc: close"
	headerRow := -1
	for i, row := range rows {
		if strings.Contains(row, wantHeader) {
			headerRow = i
			break
		}
	}
	if headerRow < 0 || headerRow+2 >= len(rows) {
		t.Fatalf("reader header missing timestamp: %q", rendered)
	}
	if strings.Contains(rows[headerRow+1], "First line") ||
		strings.ContainsAny(rows[headerRow+1], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("reader spacer must be blank: %q", rows[headerRow+1])
	}
	if !strings.Contains(rows[headerRow+2], "First line") {
		t.Fatalf("first content line must follow spacer: %q", rows[headerRow+2])
	}
	if m.longReaderVisibleHeight() != max(1, m.messageHeight()-2) {
		t.Fatal("reader scrolling height did not account for new spacer")
	}
}

func TestLongReaderHeaderNavigationReturnsToChat(t *testing.T) {
	makeReader := func() model {
		packet := interactionTestPacket("reader-navigation", strings.Repeat("reader text for scrolling. ", 200))
		packet.Long = true
		m := newMessageInteractionTestModel(100, packet)
		m.onlineCount = 2
		m.openLongReader(packet)
		return m
	}
	t.Run("splash", func(t *testing.T) {
		m := makeReader()
		updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 0})
		m = updated.(model)
		if !m.splashOpen || m.longReader != nil {
			t.Fatalf("ZEALOT header should leave reader for splash, got splash=%v reader=%v", m.splashOpen, m.longReader != nil)
		}
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.width-2, Y: 0})
		m = updated.(model)
		if m.splashOpen || m.longReader != nil || m.usersOpen {
			t.Fatal("closing splash should return to main chat")
		}
	})
	t.Run("roster", func(t *testing.T) {
		m := makeReader()
		updated, _ := m.Update(tea.MouseClickMsg{
			Button: tea.MouseLeft, X: m.width-2, Y: 0,
		})
		m = updated.(model)
		if !m.usersOpen || m.longReader != nil {
			t.Fatalf("Online header should leave reader for roster, got users=%v reader=%v", m.usersOpen, m.longReader != nil)
		}
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.width-2, Y: 0})
		m = updated.(model)
		if m.usersOpen || m.longReader != nil || m.splashOpen {
			t.Fatal("closing roster should return to main chat")
		}
	})
}

func TestLongReaderCloseButtonAndScrollbarColors(t *testing.T) {
	packet := interactionTestPacket("reader-close", strings.Repeat("content words for a scrolling reader. ", 150))
	packet.Long = true
	m := newMessageInteractionTestModel(100, packet)
	m.openLongReader(packet)
	rendered := m.renderLongReaderPanel()
	if !strings.Contains(stripANSISequences(rendered), "[X]") {
		t.Fatal("reader panel does not show close button")
	}
	// Both the main chat's rail and the long reader's rail use the theme's
	// border color, rather than the muted system-label color.
	bg := lipgloss.Color(m.cfg.Theme.Background)
	wantRail := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.BorderColor)).
		Background(bg).Render("^")
	if !strings.Contains(rendered, wantRail) {
		t.Errorf("reader scrollbar is not styled like the chat scrollbar")
	}
	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft, X: m.longReaderCloseX()+1, Y: 2,
	})
	m = updated.(model)
	if m.longReader != nil || m.usersOpen || m.splashOpen {
		t.Fatal("reader [X] should return to main chat")
	}
}
