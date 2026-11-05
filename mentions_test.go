package main

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestMentionSpansMatchCompleteRoomUsernames(t *testing.T) {
	text := "Hi @alice, @Bob Smith! @alice2 x@Alice @Alice_2x and mail@example.com"
	users := []string{"Alice", "Bob Smith", "alice2", "Alice_2"}

	spans := mentionSpans(text, users)
	got := make([]string, 0, len(spans))
	for _, span := range spans {
		got = append(got, span.Name)
	}
	want := []string{"Alice", "Bob Smith", "alice2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mention names = %#v, want %#v", got, want)
	}
}

func TestExtractMentionNamesCanonicalizesAndDeduplicates(t *testing.T) {
	got := extractMentionNames("@ALICE hi @alice and @Bob", []string{"Alice", "Bob"})
	want := []string{"Alice", "Bob"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractMentionNames() = %#v, want %#v", got, want)
	}
}

func TestMentionRenderingPreservesVisibleText(t *testing.T) {
	body := lipgloss.NewStyle().Foreground(lipgloss.Color("#DDDDDD")).Background(lipgloss.Color("#202020"))
	mention := lipgloss.NewStyle().Foreground(lipgloss.Color("#7CFFCB")).Background(lipgloss.Color("#202020")).Bold(true)
	text := "hello @Alice, please meet @Bob Smith"
	rendered := renderMentionBody(text, []string{"Alice", "Bob Smith"}, body, mention, false, 0)
	if got := stripANSISequences(rendered); got != text {
		t.Fatalf("rendered visible text = %q, want %q", got, text)
	}
	if !strings.Contains(rendered, "\x1b[") && lipgloss.ColorProfile() != lipgloss.NoColor {
		t.Fatal("mention rendering did not emit any styling sequences")
	}
}

func TestComposerMentionStylingPreservesVisibleText(t *testing.T) {
	cfg := defaultConfig()
	style := makeStyles(cfg)
	source := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.TextColor)).
		Background(lipgloss.Color(cfg.Background)).
		Render("say @Alice hello")
	rendered := styleANSIVisibleMentions(source, []string{"Alice"}, lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.AccentColor)).
		Background(lipgloss.Color(cfg.Background)).
		Bold(true))
	if got := stripANSISequences(rendered); got != "say @Alice hello" {
		t.Fatalf("composer visible text = %q, want %q", got, "say @Alice hello")
	}
	_ = style
}

func TestMessageBackgroundHighlightsOnlyMentionRecipients(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.user.Name = "Bob"
	packet := interactionTestPacket("mention", "hello")
	packet.Mentions = []string{"bob"}

	if got := string(m.messageBackground(packet)); got != mentionHighlightBackground {
		t.Fatalf("recipient background = %q, want %q", got, mentionHighlightBackground)
	}
	packet.Mentions = []string{"Alice"}
	if got := string(m.messageBackground(packet)); got != m.cfg.Background {
		t.Fatalf("non-recipient background = %q, want configured background %q", got, m.cfg.Background)
	}
}

func TestMessageUsernameHitboxMatchesWideAndCompactHeaders(t *testing.T) {
	packet := interactionTestPacket("username-hitbox", "body text")

	wide := newMessageInteractionTestModel(80, packet)
	left := 1
	nameStart := left + lipgloss.Width("["+packet.Time.Local().Format("15:04")+"]") + 1
	if got := wide.messageUsernameAt(nameStart, 2); got != packet.User.Name {
		t.Fatalf("wide username hitbox returned %q, want %q", got, packet.User.Name)
	}
	if got := wide.messageUsernameAt(nameStart+lipgloss.Width(packet.User.Name), 2); got != "" {
		t.Fatalf("wide hitbox extended past username: %q", got)
	}

	compact := newMessageInteractionTestModel(48, packet)
	if got := compact.messageUsernameAt(1, 2); got != packet.User.Name {
		t.Fatalf("compact username hitbox returned %q, want %q", got, packet.User.Name)
	}
	if got := compact.messageUsernameAt(lipgloss.Width(packet.User.Name)+1, 2); got != "" {
		t.Fatalf("compact hitbox extended past username: %q", got)
	}
}

func TestRosterUsernameHitboxMatchesRenderedName(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.usersOpen = true
	m.onlineUsers = []User{{Name: "Alice", Color: "#7CFFCB"}}
	if got := m.userNameAt(5, 3); got != "Alice" {
		t.Fatalf("roster username hitbox returned %q, want Alice", got)
	}
	if got := m.userNameAt(10, 3); got != "" {
		t.Fatalf("roster username hitbox extended past name: %q", got)
	}
}

func TestClickingMessageUsernameInsertsMentionAtCursor(t *testing.T) {
	packet := interactionTestPacket("mention-click", "body")
	m := newMessageInteractionTestModel(80, packet)
	m.resizeInput()
	m.input.SetValue("hello")
	m.input.MoveToEnd()

	nameStart := 1 + lipgloss.Width("["+packet.Time.Local().Format("15:04")+"]") + 1
	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      nameStart,
		Y:      2,
	})
	got := updated.(model)
	if want := "hello @Alice "; got.input.Value() != want {
		t.Fatalf("composer after username click = %q, want %q", got.input.Value(), want)
	}
}

func TestClickingRosterUsernameInsertsMention(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.resizeInput()
	m.usersOpen = true
	m.onlineUsers = []User{{Name: "Alice", Color: "#7CFFCB"}}
	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      5,
		Y:      3,
	})
	got := updated.(model)
	if want := "@Alice "; got.input.Value() != want {
		t.Fatalf("composer after roster username click = %q, want %q", got.input.Value(), want)
	}
}
