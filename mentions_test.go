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
}

func TestComposerMentionStylingPreservesVisibleText(t *testing.T) {
	cfg := defaultConfig()
	source := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.Theme.TextColor)).
		Background(lipgloss.Color(cfg.Theme.Background)).
		Render("say @Alice hello")
	rendered := styleANSIVisibleMentions(source, []string{"Alice"}, lipgloss.NewStyle().
		Foreground(lipgloss.Color(cfg.Theme.AccentColor)).
		Background(lipgloss.Color(cfg.Theme.Background)).
		Bold(true))
	if got := stripANSISequences(rendered); got != "say @Alice hello" {
		t.Fatalf("composer visible text = %q, want %q", got, "say @Alice hello")
	}
}

func TestComposerMentionRestoresThemeColorsAfterHighlightedName(t *testing.T) {
	for _, theme := range []Theme{
		defaultTheme(),
		{
			Background: "#07366B", TextColor: "#F5F3E9",
			AccentColor: "#FFD348",
		},
	} {
		base := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.TextColor)).
			Background(lipgloss.Color(theme.Background))
		highlight := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.AccentColor)).
			Background(lipgloss.Color(theme.Background)).
			Bold(true)

		source := base.Render("say @Alice and @Bob done")
		rendered := styleANSIVisibleMentions(source, []string{"Alice", "Bob"}, highlight)
		if got := stripANSISequences(rendered); got != "say @Alice and @Bob done" {
			t.Fatalf("mention text changed: %q", got)
		}
		// The original textarea's SGR colors must be restored between
		// mentions and after the final one, not left at terminal defaults.
		originalSGR := ansiSGRStateBefore(source, strings.Index(source, "@Alice"))
		if originalSGR == "" {
			t.Fatal("test source had no ANSI theme colors")
		}
		for _, fragment := range []string{" and ", " done"} {
			previous := "@Alice"
			if fragment == " done" {
				previous = "@Bob"
			}
			at := strings.Index(rendered, previous)
			if at < 0 {
				t.Fatalf("missing mention %q", previous)
			}
			following := rendered[at+len(previous):]
			next := strings.Index(following, fragment)
			if next < 0 || !strings.Contains(following[:next], originalSGR) {
				t.Fatalf("composer colors were not restored after %s: %q", previous, following)
			}
		}
	}
}

func TestComposerMentionRestoresSelectionSGR(t *testing.T) {
	source := "\x1b[48;2;35;122;55m\x1b[38;2;245;243;233m@Alice selected"
	highlight := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD348")).
		Background(lipgloss.Color("#07366B"))
	rendered := styleANSIVisibleMentions(source, []string{"Alice"}, highlight)
	restored := ansiSGRStateBefore(source, strings.Index(source, "@Alice"))
	if !strings.Contains(rendered, "@Alice") || !strings.Contains(rendered, restored+" selected") {
		t.Fatalf("selection styling was not restored after mention: %q", rendered)
	}
}

func TestMessageBackgroundHighlightsOnlyMentionRecipients(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	m.user.Name = "Bob"
	m.cfg.Theme.MentionBackgroundColor = "#AABBCC"
	packet := interactionTestPacket("mention", "hello")
	packet.Mentions = []string{"bob"}

	if got := m.messageBackground(packet); got != lipgloss.Color("#AABBCC") {
		t.Fatalf("recipient background = %v, want %v", got, lipgloss.Color("#AABBCC"))
	}
	packet.Mentions = []string{"Alice"}
	if got := m.messageBackground(packet); got != lipgloss.Color(m.cfg.Theme.Background) {
		t.Fatalf("non-recipient background = %v, want configured background %v", got, lipgloss.Color(m.cfg.Theme.Background))
	}
}

func TestMentionStyleUsesThemeHoverColor(t *testing.T) {
	m := newMessageInteractionTestModel(80)
	background := lipgloss.Color(m.cfg.Theme.Background)

	hovered := m.mentionStyle(background, true).Render("@Alice")
	wantHovered := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.HoverTextColor)).
		Background(background).
		Bold(true).
		Render("@Alice")
	if hovered != wantHovered {
		t.Fatalf("hovered mention style does not use theme hover color")
	}

	normal := m.mentionStyle(background, false).Render("@Alice")
	wantNormal := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
		Background(background).
		Bold(true).
		Render("@Alice")
	if normal != wantNormal {
		t.Fatalf("normal mention style does not use theme accent color")
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
