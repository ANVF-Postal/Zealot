package main

import (
	"fmt"
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
		styles:   makeStyles(cfg.Theme),
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
	empty := newMessageInteractionTestModel(80)
	if got := empty.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("empty chat MouseMode = %v, want MouseModeCellMotion without hover targets", got)
	}

	m := newMessageInteractionTestModel(80, interactionTestPacket("hoverable", "message body"))
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

func TestUserRowsRangeMatchesFullRoster(t *testing.T) {
	m := newMessageInteractionTestModel(40)
	m.onlineUsers = []User{
		{Name: "A very long user name that wraps", Color: "#7CFFCB"},
		{Name: "Bob", Color: "#F2D66B", Typing: true},
		{Name: "Carol", Color: "#8AA4FF"},
	}
	all := m.userRows()
	if got := m.userRowCount(); got != len(all) {
		t.Fatalf("userRowCount() = %d, rendered rows = %d", got, len(all))
	}
	for start := 0; start < len(all); start++ {
		end := min(start+2, len(all))
		got := m.userRowsRange(start, end)
		if len(got) != end-start {
			t.Fatalf("userRowsRange(%d, %d) returned %d rows", start, end, len(got))
		}
		for i := range got {
			if got[i] != all[start+i] {
				t.Fatalf("userRowsRange(%d, %d)[%d] = %q, want %q", start, end, i, got[i], all[start+i])
			}
		}
	}
}

func TestUsersAnimationStopsWhenRosterIsIdle(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.usersOpen = true
	m.onlineUsers = []User{{Name: "Idle", Color: "#7CFFCB"}}

	updated, cmd := m.Update(usersAnimationMsg{})
	got := updated.(model)
	if cmd != nil {
		t.Fatal("idle online-users panel scheduled another animation tick")
	}
	if got.usersAnimationRunning {
		t.Fatal("idle online-users panel left its animation marked as running")
	}
}

func TestOpeningOnlineUsersDefersTheRosterRequest(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.onlineCount = 1
	// A nil client would panic if Update tried to send the request inline.
	updated, cmd := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      72,
		Y:      0,
	})
	got := updated.(model)
	if !got.usersOpen {
		t.Fatal("clicking the online count did not open the roster")
	}
	if cmd == nil {
		t.Fatal("opening the roster did not schedule the asynchronous refresh")
	}
}

func TestRenderCacheCalculatesInteractionHitboxesOnDemand(t *testing.T) {
	parent := interactionTestPacket("lazy-parent", "original message body")
	reply := interactionTestPacket("lazy-reply", "reply body")
	reply.ReplyTo = parent.ID
	m := newMessageInteractionTestModel(80, parent, reply)

	if len(m.messageBodyRangesByID) != 0 || len(m.messageLinkRangesByID) != 0 {
		t.Fatal("cache rebuild eagerly calculated interaction geometry for every message")
	}
	if len(m.messageRowRanges) != len(m.messages) {
		t.Fatal("absolute row index is not aligned with the message history")
	}

	body := m.messageBodyRanges(parent, m.width)[0]
	if got := m.messageIDAt(1+body.Start, 2); got != parent.ID {
		t.Fatalf("body hover returned %q, want %q", got, parent.ID)
	}
	if len(m.messageBodyRangesByID) != 1 {
		t.Fatalf("body hitbox cache contains %d messages, want only the hovered message", len(m.messageBodyRangesByID))
	}

	badge := m.messageLinkRanges(reply, m.width)[0][0]
	if got := m.messageLinkAt(1+badge.Start+1, 3); got != parent.ID {
		t.Fatalf("reply badge click returned %q, want %q", got, parent.ID)
	}
	if len(m.messageLinkRangesByID) != 1 {
		t.Fatalf("reply-link cache contains %d messages, want only the clicked message", len(m.messageLinkRangesByID))
	}
}

func TestPacketRowCountMatchesRendererAtCommonWidths(t *testing.T) {
	parent := interactionTestPacket("count-parent", strings.Repeat("word ", 18))
	reply := interactionTestPacket("count-reply", "reply body")
	reply.ReplyTo = parent.ID
	longName := interactionTestPacket("count-long-name", strings.Repeat("U", 70)+" message")
	packets := []Packet{
		interactionTestPacket("count-short", "short body"),
		interactionTestPacket("count-wrap", strings.Repeat("word ", 35)),
		interactionTestPacket("count-newlines", "first line\n\nthird line"),
		interactionTestPacket("count-long-token", strings.Repeat("x", 120)),
		longName,
		parent,
		reply,
		{Type: "system", Text: strings.Repeat("system notice ", 10), Time: parent.Time},
		{Type: "me", Text: strings.Repeat("waves ", 12), User: parent.User, Time: parent.Time},
		{Type: "letter", Text: "letter body\n"+strings.Repeat("wide text ", 12), User: parent.User, Time: parent.Time},
		{Type: "message", ID: "count-xkeyscore", Text: "XKEYSCORE", User: parent.User, Time: parent.Time, XKeyscore: 1},
		{Type: "message", ID: "count-xkeyscore-3", Text: "XKEYSCORE", User: parent.User, Time: parent.Time, XKeyscore: 3, Rainbow: true},
	}
	m := newMessageInteractionTestModel(80, packets...)
	m.rebuildMessageIndexes()
	for _, width := range []int{20, 32, 48, 49, 64, 80, 120} {
		for _, packet := range packets {
			got := m.renderPacketLineCount(packet, width)
			want := len(m.renderPacketLines(packet, width))
			if got != want {
				t.Errorf("row count for %s at width %d = %d, renderer produced %d", packet.ID, width, got, want)
			}
		}
	}
}

func TestOnlyViewportAndThreeRowBufferAreStyled(t *testing.T) {
	messages := make([]Packet, 100)
	for i := range messages {
		messages[i] = interactionTestPacket(fmt.Sprintf("virtual-%d", i), "short message")
	}
	m := newMessageInteractionTestModel(80, messages...)
	first := m.messageRowsByID[messages[0].ID]
	last := m.messageRowsByID[messages[len(messages)-1].ID]
	if m.renderedLines[first.Start] != "" {
		t.Fatal("off-screen first message was styled during initial viewport render")
	}
	if m.renderedLines[last.Start] == "" {
		t.Fatal("latest message in the viewport was not styled")
	}
	if !m.renderWindowValid || m.renderWindowStart <= first.Start {
		t.Fatalf("rendered window = %d..%d, want a bounded window near the bottom", m.renderWindowStart, m.renderWindowEnd)
	}

	m.scroll = m.maxScroll()
	m.clampScroll()
	if m.renderedLines[first.Start] == "" {
		t.Fatal("scrolling to the top did not render the first message")
	}
	if m.renderedLines[last.Start] != "" {
		t.Fatal("scrolling to the top left the off-screen latest message styled")
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
	rowStorage := &m.messageRowRanges[0]

	m.mousePositionKnown = true
	m.lastMouseX = 1 + firstBody[0].Start
	m.lastMouseY = 2
	m.refreshMessageHover()

	if m.hoveredMessageID != first.ID {
		t.Fatalf("hovered message = %q, want %q", m.hoveredMessageID, first.ID)
	}
	if !m.renderCacheValid || &m.renderedLines[0] != lineStorage ||
		&m.messageRowRanges[0] != rowStorage {
		t.Fatal("hover transition rebuilt the full message cache instead of restyling rows in place")
	}
}

func TestMessageHoverMotionUsesLatestPositionPerFrame(t *testing.T) {
	first := interactionTestPacket("hover-frame-1", "first message body")
	second := interactionTestPacket("hover-frame-2", "second message body")
	m := newMessageInteractionTestModel(80, first, second)
	m.mouseMotion = &mouseMotionState{}
	filter := mouseMotionFilter{state: m.mouseMotion}

	firstBody := m.messageBodyRanges(first, m.width)[0]
	secondBody := m.messageBodyRanges(second, m.width)[0]
	firstMotion := tea.MouseMotionMsg{X: 1 + firstBody.Start, Y: 2}
	if filter.filter(m, firstMotion) == nil {
		t.Fatal("first pointer motion was not admitted")
	}
	updated, cmd := m.Update(firstMotion)
	if cmd == nil {
		t.Fatal("first pointer motion did not schedule a frame refresh")
	}
	m = updated.(model)

	// Sweep repeatedly between rows while the first frame is pending. Every
	// intermediate event is dropped, but the final position must be retained.
	for i := 0; i < 1000; i++ {
		motion := tea.MouseMotionMsg{X: 1 + firstBody.Start, Y: 2}
		if i == 999 {
			motion = tea.MouseMotionMsg{X: 1 + secondBody.Start, Y: 3}
		}
		if got := filter.filter(m, motion); got != nil {
			t.Fatalf("motion %d escaped while a frame refresh was pending", i)
		}
	}
	generation := m.mouseMotionFrameGeneration
	updated, next := m.Update(mouseMotionFrameMsg{generation: generation})
	if next != nil {
		t.Fatal("frame refresh unexpectedly scheduled another command")
	}
	got := updated.(model)
	if got.hoveredMessageID != second.ID {
		t.Fatalf("hovered message = %q, want latest pointer target %q", got.hoveredMessageID, second.ID)
	}
}

func TestMouseMotionFilterCoalescesScrollbarDragAndPreservesActions(t *testing.T) {
	messages := make([]Packet, 40)
	for i := range messages {
		messages[i] = interactionTestPacket(fmt.Sprintf("drag-%d", i), "scrollable message")
	}
	m := newMessageInteractionTestModel(80, messages...)
	m.mouseMotion = &mouseMotionState{}
	m.scrollDragging = true
	filter := mouseMotionFilter{state: m.mouseMotion}

	_, thumbHeight, trackHeight := m.scrollbarThumb()
	maxTop := trackHeight - thumbHeight
	targetTop := maxTop / 2
	targetY := 3 + targetTop + m.scrollDragOffset
	first := tea.MouseMotionMsg{X: m.chatScrollbarX(), Y: 4, Button: tea.MouseLeft}
	if filter.filter(m, first) == nil {
		t.Fatal("first scrollbar drag motion was filtered")
	}
	updated, frameCmd := m.Update(first)
	if frameCmd == nil {
		t.Fatal("first scrollbar drag motion did not schedule a frame refresh")
	}
	m = updated.(model)

	for i := 0; i < 1000; i++ {
		motion := tea.MouseMotionMsg{X: m.chatScrollbarX(), Y: 5 + i%10, Button: tea.MouseLeft}
		if i == 999 {
			motion.Y = targetY
		}
		if filter.filter(m, motion) != nil {
			t.Fatalf("scrollbar drag motion %d was not coalesced", i)
		}
	}
	if x, y, known := m.mouseMotion.position(); !known || x != m.chatScrollbarX() || y != targetY {
		t.Fatalf("latest drag position = (%d, %d, %t), want (%d, %d, true)", x, y, known, m.chatScrollbarX(), targetY)
	}
	generation := m.mouseMotionFrameGeneration
	updated, next := m.Update(mouseMotionFrameMsg{generation: generation})
	if next != nil {
		t.Fatal("drag frame unexpectedly scheduled an additional command")
	}
	got := updated.(model)
	expected := m
	expected.setScrollFromThumbTopForLineCount(targetTop, len(expected.renderMessageLines()))
	if got.scroll != expected.scroll || got.scroll == 0 || got.scroll == got.maxScroll() {
		t.Fatalf("coalesced drag scroll = %d, expected middle position %d (range 0..%d)", got.scroll, expected.scroll, got.maxScroll())
	}

	if filter.filter(got, tea.MouseClickMsg{Button: tea.MouseLeft, X: 79, Y: targetY}) == nil {
		t.Fatal("click was filtered while a motion frame was pending")
	}
	if filter.filter(got, tea.MouseReleaseMsg{}) == nil {
		t.Fatal("mouse release was filtered while a motion frame was pending")
	}
}

func TestMouseReleaseFlushesLatestCoalescedDragPosition(t *testing.T) {
	messages := make([]Packet, 40)
	for i := range messages {
		messages[i] = interactionTestPacket(fmt.Sprintf("release-drag-%d", i), "scrollable message")
	}
	m := newMessageInteractionTestModel(80, messages...)
	m.mouseMotion = &mouseMotionState{}
	m.scrollDragging = true
	filter := mouseMotionFilter{state: m.mouseMotion}

	_, thumbHeight, trackHeight := m.scrollbarThumb()
	targetTop := (trackHeight - thumbHeight) / 2
	targetY := 3 + targetTop
	first := tea.MouseMotionMsg{X: m.chatScrollbarX(), Y: 4, Button: tea.MouseLeft}
	if filter.filter(m, first) == nil {
		t.Fatal("first drag event was filtered")
	}
	updated, cmd := m.Update(first)
	if cmd == nil {
		t.Fatal("first drag event did not schedule a frame")
	}
	m = updated.(model)

	latest := tea.MouseMotionMsg{X: m.chatScrollbarX(), Y: targetY, Button: tea.MouseLeft}
	if filter.filter(m, latest) != nil {
		t.Fatal("intermediate drag event was not coalesced")
	}
	updated, _ = m.Update(tea.MouseReleaseMsg{})
	got := updated.(model)
	if got.scrollDragging {
		t.Fatal("mouse release left scrollbar dragging active")
	}
	expected := m
	expected.setScrollFromThumbTopForLineCount(targetTop, len(expected.renderMessageLines()))
	if got.scroll != expected.scroll {
		t.Fatalf("release scroll = %d, want latest dragged position %d", got.scroll, expected.scroll)
	}

	// The already-scheduled frame is still valid, but must not resume dragging.
	generation := got.mouseMotionFrameGeneration
	updated, _ = got.Update(mouseMotionFrameMsg{generation: generation})
	if after := updated.(model); after.scroll != got.scroll || after.scrollDragging {
		t.Fatalf("post-release frame changed scroll state: scroll=%d dragging=%t", after.scroll, after.scrollDragging)
	}
}

func TestHeaderClickClearsMessageHoverImmediately(t *testing.T) {
	packet := interactionTestPacket("hover-header", "message body")
	m := newMessageInteractionTestModel(80, packet)
	m.mousePositionKnown = true
	m.lastMouseX = 1 + m.messageBodyRanges(packet, m.width)[0].Start
	m.lastMouseY = 2
	m.refreshMessageHover()
	if m.hoveredMessageID != packet.ID {
		t.Fatal("test setup did not establish a hovered message")
	}

	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      0,
		Y:      0,
	})
	got := updated.(model)
	if got.hoveredMessageID != "" {
		t.Fatal("header click did not clear message hover immediately")
	}
	if !got.splashOpen {
		t.Fatal("header click did not open the splash panel")
	}
}

func TestSetScrollFromThumbTopUsesFullRangeEndpoints(t *testing.T) {
	messages := make([]Packet, 40)
	for i := range messages {
		messages[i] = interactionTestPacket(
			fmt.Sprintf("scroll-%d", i),
			"short line",
		)
	}
	m := newMessageInteractionTestModel(80, messages...)
	if maxScroll := m.maxScroll(); maxScroll < 1 {
		t.Fatalf("test history only provides %d scroll rows; want a scrollable history", maxScroll)
	}
	_, thumbHeight, trackHeight := m.scrollbarThumb()
	maxTop := trackHeight - thumbHeight
	if maxTop < 1 {
		t.Fatalf("test scrollbar has no draggable range: thumb=%d track=%d", thumbHeight, trackHeight)
	}

	m.setScrollFromThumbTop(-100)
	if m.scroll != m.maxScroll() {
		t.Fatalf("top clamp scroll = %d, want maximum %d", m.scroll, m.maxScroll())
	}
	m.setScrollFromThumbTop(maxTop + 100)
	if m.scroll != 0 {
		t.Fatalf("bottom clamp scroll = %d, want 0", m.scroll)
	}
}

func TestTypingClearTimerIsCoalescedAcrossKeystrokes(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	firstCmd := m.scheduleTypingClear()
	if firstCmd == nil {
		t.Fatal("first key did not schedule a typing clear timer")
	}
	firstGeneration := m.typingGeneration

	secondCmd := m.scheduleTypingClear()
	if secondCmd != nil {
		t.Fatal("a second key scheduled an overlapping typing clear timer")
	}
	if !m.typingTimerPending {
		t.Fatal("typing timeout was not kept pending across the burst")
	}
	if m.typingGeneration <= firstGeneration {
		t.Fatal("later key did not move the typing deadline generation")
	}

	m.isTyping = true
	updated, rearmCmd := m.Update(typingClearMsg{generation: firstGeneration})
	got := updated.(model)
	if rearmCmd == nil {
		t.Fatal("stale first timer did not re-arm for the latest key deadline")
	}
	if !got.typingTimerPending {
		t.Fatal("re-arming the timer cleared the pending state")
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
	widePlain := stripANSISequences(strings.Join(wide, "\n"))
	if len(wide) == 0 || !strings.Contains(widePlain, "Alice[Re]:") || strings.Contains(widePlain, "[Replied]") || strings.Contains(widePlain, "[Re] :") {
		t.Fatalf("wide reply badge should be adjacent to username and colon: %q", widePlain)
	}
	compact := m.renderPacketLines(reply, 48)
	compactPlain := stripANSISequences(strings.Join(compact, "\n"))
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
	_ = m.cachedBodyRangesForMessage(parent.ID, parentRowsBefore.End-parentRowsBefore.Start)
	_ = m.cachedLinkRangesForMessage(parent.ID, parentRowsBefore.End-parentRowsBefore.Start)
	reply := interactionTestPacket("incremental-reply", "reply body")
	reply.ReplyTo = parent.ID
	m.appendMessage(reply)

	if _, ok := m.messageBodyRangesByID[parent.ID]; ok {
		t.Fatal("appending a reply kept stale parent body hitboxes")
	}
	if _, ok := m.messageLinkRangesByID[parent.ID]; ok {
		t.Fatal("appending a reply kept stale parent link hitboxes")
	}
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
	if len(m.messageRowRanges) != len(m.messages) || len(m.messageRowsByID) != len(m.messages) {
		t.Fatal("absolute message row index is not aligned with the message history")
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
	if packet, rows, ok := m.messageAtRow(parentRows.End-1); !ok || packet.ID != parent.ID || rows != parentRows {
		t.Fatalf("overflow marker row mapped to %q at %#v, want parent %q at %#v", packet.ID, rows, parent.ID, parentRows)
	}
	if packet, rows, ok := m.messageAtRow(followingRows.Start); !ok || packet.ID != following.ID || rows != followingRows {
		t.Fatalf("following message row mapped to %q at %#v, want %q at %#v", packet.ID, rows, following.ID, followingRows)
	}
	if !strings.Contains(m.renderedLines[parentRows.End-1], "[1]") {
		t.Fatalf("overflow row does not display [1]: %q", m.renderedLines[parentRows.End-1])
	}
	if len(m.messageRowRanges) != len(m.messages) || len(m.messageRowsByID) != len(m.messages) {
		t.Fatal("message row index lost alignment after inserting an overflow row")
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
	firstRow := stripANSISequences(lines[0])
	if !strings.Contains(firstRow, "short message [1]") {
		t.Fatalf("reply marker should stay at the end of the body on the first row: %q", firstRow)
	}
	timestampRow := strings.TrimSpace(stripANSISequences(lines[1]))
	wantTimestamp := "[" + parent.Time.Local().Format("15:04") + "]"
	if timestampRow != wantTimestamp {
		t.Fatalf("timestamp-only row = %q, want %q (separate from the reply marker)", timestampRow, wantTimestamp)
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

func TestConnectionCloseDoesNotRenderDisconnectedDuringQuit(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.connected = true
	m.status = "Connected"
	m.quitting = true

	updated, cmd := m.Update(connectionClosedMsg{})
	got := updated.(model)
	if !got.connected {
		t.Fatal("intentional quit changed connected state before the TUI exited")
	}
	if got.status == "Disconnected" {
		t.Fatal("intentional quit rendered a transient Disconnected status")
	}
	if cmd == nil {
		t.Fatal("connection close during intentional quit should finish quitting")
	}
}

func TestUnexpectedConnectionCloseStillShowsDisconnected(t *testing.T) {
	m := newMessageInteractionTestModel(80)

	updated, cmd := m.Update(connectionClosedMsg{})
	got := updated.(model)
	if got.connected || got.status != "Disconnected" {
		t.Fatalf("unexpected close state = connected:%t status:%q, want disconnected", got.connected, got.status)
	}
	if cmd != nil {
		t.Fatal("unexpected connection close should not quit the application")
	}
}
