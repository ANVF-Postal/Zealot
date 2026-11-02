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

func TestPanelViewsDisablePassiveMouseMotion(t *testing.T) {
	m := newMessageInteractionTestModel(80)

	if got := m.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Fatalf("main chat MouseMode = %v, want MouseModeAllMotion for message hover", got)
	}

	m.splashOpen = true
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("splash MouseMode = %v, want MouseModeCellMotion without passive hover", got)
	}

	m.splashOpen = false
	m.usersOpen = true
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("online users MouseMode = %v, want MouseModeCellMotion without passive hover", got)
	}
}

func TestMessageHoverRestylesOnlyAffectedRows(t *testing.T) {
	first := interactionTestPacket("hover-1", "hover this body")
	second := interactionTestPacket("hover-2", "another message")
	m := newMessageInteractionTestModel(80, first, second)
	if len(m.renderedLines) == 0 {
		t.Fatal("expected cached message rows")
	}

	firstBody := m.messageBodyRanges(first, m.width)
	if len(firstBody) == 0 || firstBody[0].End <= firstBody[0].Start {
		t.Fatalf("expected a body hitbox, got %#v", firstBody)
	}
	lineStorage := &m.renderedLines[0]
	idStorage := &m.renderedMessageIDs[0]
	bodyStorage := &m.renderedMessageBodyRanges[0]

	m.mousePositionKnown = true
	m.lastMouseX = 1 + firstBody[0].Start
	m.lastMouseY = 2
	m.refreshMessageHover()

	if m.hoveredMessageID != first.ID {
		t.Fatalf("hovered message = %q, want %q", m.hoveredMessageID, first.ID)
	}
	if !m.renderCacheValid || &m.renderedLines[0] != lineStorage ||
		&m.renderedMessageIDs[0] != idStorage || &m.renderedMessageBodyRanges[0] != bodyStorage {
		t.Fatal("hover transition rebuilt the full message cache instead of restyling rows in place")
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

func TestReplyBadgesUseShortForms(t *testing.T) {
	parent := interactionTestPacket("badge-parent", "original")
	reply := interactionTestPacket("badge-reply", "reply")
	reply.ReplyTo = parent.ID
	m := newMessageInteractionTestModel(80, parent, reply)

	wide := m.renderPacketLines(reply, 80)
	widePlain := lipgloss.StripANSI(strings.Join(wide, "\n"))
	if len(wide) == 0 || !strings.Contains(widePlain, "Alice[Re]:") || strings.Contains(widePlain, "[Replied]") || strings.Contains(widePlain, "[Re] :") {
		t.Fatalf("wide reply badge should be adjacent to username and colon: %q", widePlain)
	}
	compact := m.renderPacketLines(reply, 48)
	compactPlain := lipgloss.StripANSI(strings.Join(compact, "\n"))
	if len(compact) == 0 || !strings.Contains(compactPlain, "Alice[R]:") || strings.Contains(compactPlain, "[R] Bob") || strings.Contains(compactPlain, "[Re]") {
		t.Fatalf("compact reply badge should follow username: %q", compactPlain)
	}
	compactLinks := m.messageLinkRanges(reply, 48)
	if len(compactLinks) == 0 || len(compactLinks[0]) == 0 ||
		compactLinks[0][0].Start != lipgloss.Width("Alice") || compactLinks[0][0].End != lipgloss.Width("Alice[R]") {
		t.Fatalf("compact [R] hitbox should follow username and cover only label: %#v", compactLinks)
	}

	longNameReply := reply
	longNameReply.ID = "badge-long-name-reply"
	longNameReply.User.Name = strings.Repeat("U", 70)
	longNameModel := newMessageInteractionTestModel(80, parent, longNameReply)
	fallback := longNameModel.renderPacketLines(longNameReply, 80)
	if len(fallback) == 0 || !strings.Contains(fallback[0], "[R]") || strings.Contains(fallback[0], "[Re]") {
		t.Fatalf("wide terminal with compact fallback should use [R]: %q", fallback)
	}
	fallbackLinks := longNameModel.messageLinkRanges(longNameReply, 80)
	if len(fallbackLinks) == 0 || len(fallbackLinks[0]) == 0 {
		t.Fatalf("compact fallback [R] hitbox missing: %#v", fallbackLinks)
	}
	fallbackName := wrapText(longNameReply.User.Name, chatContentWidth(80)-2-lipgloss.Width("[R]"))
	wantRow := len(fallbackName)-1
	if wantRow >= len(fallbackLinks) || len(fallbackLinks[wantRow]) == 0 {
		t.Fatalf("compact fallback [R] hitbox missing on final username row: %#v", fallbackLinks)
	}
	if fallbackLinks[wantRow][0].Start != lipgloss.Width(fallbackName[wantRow]) ||
		fallbackLinks[wantRow][0].End != lipgloss.Width(fallbackName[wantRow])+lipgloss.Width("[R]") {
		t.Fatalf("compact fallback [R] hitbox should follow final username row: %#v", fallbackLinks)
	}
}

func TestAppendingReplyUpdatesOnlyParentRowsAndAppendsReply(t *testing.T) {
	parent := interactionTestPacket("incremental-parent", "parent body")
	m := newMessageInteractionTestModel(80, parent)
	parentRowsBefore, ok := m.messageRowsByID[parent.ID]
	if !ok {
		t.Fatal("parent should be indexed in the rendered row map")
	}
	reply := interactionTestPacket("incremental-reply", "reply body")
	reply.ReplyTo = parent.ID
	m.appendMessage(reply)

	if !m.renderCacheValid {
		t.Fatal("reply append should leave the render cache valid")
	}
	parentRowsAfter := m.messageRowsByID[parent.ID]
	if parentRowsAfter.End-parentRowsAfter.Start != parentRowsBefore.End-parentRowsBefore.Start {
		t.Fatalf("parent row count changed unexpectedly: before=%#v after=%#v", parentRowsBefore, parentRowsAfter)
	}
	if !strings.Contains(strings.Join(m.renderedLines[parentRowsAfter.Start:parentRowsAfter.End], "\n"), "[1]") {
		t.Fatal("parent message should show [1] after reply append")
	}
	if rows, ok := m.messageRowsByID[reply.ID]; !ok || rows.Start != parentRowsAfter.End {
		t.Fatalf("reply row range = %#v (present %v), want append immediately after parent", rows, ok)
	}
	if len(m.renderedMessageIDs) != len(m.renderedLines) ||
		len(m.renderedMessageBodyRanges) != len(m.renderedLines) ||
		len(m.renderedMessageLinks) != len(m.renderedLines) {
		t.Fatal("rendered row metadata is not aligned with rendered lines")
	}
}

func TestAppendingReplyShiftsCachedRowsWhenMarkerNeedsOverflowRow(t *testing.T) {
	parent := interactionTestPacket("overflow-parent", strings.Repeat("x", 60))
	following := interactionTestPacket("overflow-following", "following message")
	m := newMessageInteractionTestModel(80, parent, following)

	reply := interactionTestPacket("overflow-reply", "reply body")
	reply.ReplyTo = parent.ID
	m.appendMessage(reply)

	parentRows := m.messageRowsByID[parent.ID]
	followingRows := m.messageRowsByID[following.ID]
	replyRows := m.messageRowsByID[reply.ID]
	if parentRows.End-parentRows.Start != 2 {
		t.Fatalf("parent should gain an overflow marker row: %#v", parentRows)
	}
	if followingRows.Start != parentRows.End || replyRows.Start != followingRows.End {
		t.Fatalf("cached ranges overlap or skipped rows: parent=%#v following=%#v reply=%#v", parentRows, followingRows, replyRows)
	}
	if !strings.Contains(m.renderedLines[parentRows.End-1], "[1]") {
		t.Fatalf("overflow row does not display [1]: %q", m.renderedLines[parentRows.End-1])
	}
	if len(m.renderedMessageIDs) != len(m.renderedLines) ||
		len(m.renderedMessageBodyRanges) != len(m.renderedLines) ||
		len(m.renderedMessageLinks) != len(m.renderedLines) {
		t.Fatal("render metadata lost alignment after inserting an overflow row")
	}
}

func TestCompactReplyMarkerStaysBesideMessageWhenThereIsRoom(t *testing.T) {
	parent := interactionTestPacket("compact-marker-parent", "short message")
	reply := interactionTestPacket("compact-marker-reply", "reply")
	reply.ReplyTo = parent.ID
	m := newMessageInteractionTestModel(48, parent, reply)

	lines := m.renderPacketLines(parent, m.width)
	if len(lines) < 2 {
		t.Fatalf("expected compact message header and timestamp rows, got %#v", lines)
	}
	firstRow := lipgloss.StripANSI(lines[0])
	if !strings.Contains(firstRow, "short message [1]") {
		t.Fatalf("reply marker should stay at the end of the body on the first row: %q", firstRow)
	}
	timestampRow := strings.TrimSpace(lipgloss.StripANSI(lines[1]))
	if timestampRow != "[15:04]" {
		t.Fatalf("timestamp-only row should remain separate from the reply marker: %q", timestampRow)
	}

	links := m.messageLinkRanges(parent, m.width)
	if len(links) == 0 || len(links[0]) != 1 || links[0][0].TargetID != reply.ID {
		t.Fatalf("compact inline marker should remain clickable on the body row: %#v", links)
	}
}

func TestReplyMarkersWrapWithoutTakingScrollbarColumn(t *testing.T) {
	parent := interactionTestPacket("parent-wrap", strings.Repeat("x", 60))
	first := interactionTestPacket("reply-wrap-1", "first reply")
	first.ReplyTo = parent.ID
	second := interactionTestPacket("reply-wrap-2", "second reply")
	second.ReplyTo = parent.ID
	m := newMessageInteractionTestModel(80, parent, first, second)

	lines := m.renderPacketLines(parent, m.width)
	if len(lines) != 2 {
		t.Fatalf("got %d rows, want body plus one marker row", len(lines))
	}
	markerRow := lines[1]
	if !strings.Contains(markerRow, "[1]") || !strings.Contains(markerRow, "[2]") {
		t.Fatalf("overflow row should contain both markers in arrival order: %q", markerRow)
	}
	if got, want := lipgloss.Width(markerRow), chatContentWidth(m.width); got != want {
		t.Fatalf("marker row width = %d, want %d so scrollbar stays in its own column", got, want)
	}

	links := m.messageLinkRanges(parent, m.width)
	if len(links) != 2 || len(links[1]) != 2 {
		t.Fatalf("overflow marker hitboxes = %#v, want two links on the marker row", links)
	}
	if links[1][0].TargetID != first.ID || links[1][1].TargetID != second.ID {
		t.Fatalf("overflow marker targets = %#v, want arrival order", links[1])
	}
	if links[1][0].Start != 0 || links[1][1].Start != 4 {
		t.Fatalf("overflow marker columns = %#v, want [1] at 0 and [2] at 4", links[1])
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
	if got, want := badge.End-badge.Start, lipgloss.Width("[Re]"); got != want {
		t.Fatalf("[Re] hitbox width = %d, want %d (do not include following space)", got, want)
	}
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
