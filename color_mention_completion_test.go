package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestColorCommandAcceptsBareAndMixedCaseHex(t *testing.T) {
	cases := map[string]string{
		"abcdef": "#ABCDEF",
		"AbCdEf": "#ABCDEF",
		"#aBcDeF": "#ABCDEF",
		"00ff11": "#00FF11",
		"18": "18",
		"255": "255",
	}
	for input, want := range cases {
		got, ok := normalizeUserColor(input)
		if !ok || got != want || !validUserColor(got) {
			t.Errorf("normalizeUserColor(%q) = %q, %v; want %q", input, got, ok, want)
		}
	}
	for _, invalid := range []string{"", "#abc", "gggggg", "256", "-1", "1234567"} {
		if _, ok := normalizeUserColor(invalid); ok { t.Errorf("accepted invalid color %q", invalid) }
	}
}

func TestColorChangeSystemMessageRendersTwoSwatches(t *testing.T) {
	m := newMessageInteractionTestModel(95)
	packet := Packet{
		Type: "system", Time: time.Now(),
		Text: "Alice changed their color.",
		ColorBefore: "#ff2200", ColorAfter: "#00aaee",
	}
	rows := m.renderPacketLines(packet, m.width)
	visible := stripANSISequences(strings.Join(rows, "\n"))
	if !strings.Contains(visible, "Alice changed their color. █ → █") {
		t.Fatalf("color change swatches missing from announcement: %q", visible)
	}
	if got, want := len(rows), m.renderPacketLineCount(packet, m.width); got != want {
		t.Fatalf("color message height = %d, count = %d", got, want)
	}
	if !strings.Contains(strings.Join(rows, "\n"), lipgloss.NewStyle().
		Foreground(lipgloss.Color(packet.ColorBefore)).
		Background(lipgloss.Color(m.cfg.Theme.Background)).Render("█")) {
		t.Fatal("old color swatch did not use old color")
	}
	if !strings.Contains(strings.Join(rows, "\n"), lipgloss.NewStyle().
		Foreground(lipgloss.Color(packet.ColorAfter)).
		Background(lipgloss.Color(m.cfg.Theme.Background)).Render("█")) {
		t.Fatal("new color swatch did not use new color")
	}
	packet.ColorAfter = "\x1b[31m"
	if colorChangeTextSuffix(packet) != "" { t.Fatal("unsafe color became a swatch") }
}

func TestMentionTabCompletesAndCyclesOriginalPrefix(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.mentionUsernames = []string{"Alice", "David", "Daria", "Dom", "Ed"}
	m.input.SetValue("hello @d")
	m.input.MoveToEnd()
	for i, want := range []string{"hello @David", "hello @Daria", "hello @Dom", "hello @David"} {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = updated.(model)
		if m.input.Value() != want {
			t.Fatalf("Tab %d: got %q, want %q", i+1, m.input.Value(), want)
		}
	}
	// Editing the draft resets the cycling context.
	updated, _ := m.Update(tea.KeyPressMsg{Text:"!"})
	m = updated.(model)
	if len(m.mentionTabMatches) != 0 { t.Fatal("typing did not clear Tab cycle") }
	m.input.SetValue("ping @DA")
	m.input.MoveToEnd()
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(model)
	if got := m.input.Value(); got != "ping @David" && got != "ping @Daria" {
		t.Fatalf("case-insensitive completion failed: %q", got)
	}
}

func TestMentionTabIgnoresUnmatchedAndNonMentionText(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.mentionUsernames = []string{"Daria"}
	for _, draft := range []string{"hello world", "test@example.com", "hello @zzz", "hello @d text"} {
		m.resetMentionTabCompletion()
		m.input.SetValue(draft)
		m.input.MoveToEnd()
		if m.completeMentionTab() { t.Fatalf("unexpected completion for %q", draft) }
		if m.input.Value() != draft { t.Fatalf("completion mutated %q", draft) }
	}
	m.input.SetValue("@d")
	m.input.MoveToEnd()
	if !m.completeMentionTab() || m.input.Value() != "@Daria" {
		t.Fatalf("single matching username not completed: %q", m.input.Value())
	}
}

func TestColorCommandSavesAndSendsCanonicalHex(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newMessageInteractionTestModel(95)
	m.cfg.UserColor = "#112233"
	m.user = User{Name: "Tester", Color: "#112233"}
	m.room = "lobby"
	var outgoing bytes.Buffer
	m.client = &wireClient{enc: json.NewEncoder(&outgoing)}
	m.sendMessage("/color dEaDbE", "")
	if m.cfg.UserColor != "#DEADBE" || m.user.Color != "#DEADBE" {
		t.Fatalf("color command did not normalize user color: %+v", m.user)
	}
	var packet Packet
	if err := json.NewDecoder(&outgoing).Decode(&packet); err != nil { t.Fatal(err) }
	if packet.Type != "color" || packet.User.Color != "#DEADBE" {
		t.Fatalf("wrong color update packet: %+v", packet)
	}
	stored, err := os.ReadFile(configPath())
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(stored), "user_color = \"#DEADBE\"") {
		t.Fatalf("canonical color was not saved: %s", stored)
	}
}

func TestColorChangeSwatchesKeepColorsAfterWrapping(t *testing.T) {
	m := newMessageInteractionTestModel(26)
	packet := Packet{Type: "system", Time: time.Now(),
		Text: "Alice changed their color.",
		ColorBefore: "#123456", ColorAfter: "#FEDCBA",
	}
	rows := strings.Join(m.renderPacketLines(packet, m.width), "\n")
	for _, color := range []string{packet.ColorBefore, packet.ColorAfter} {
		styled := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).
			Background(lipgloss.Color(m.cfg.Theme.Background)).Render("█")
		if !strings.Contains(rows, styled) {
			t.Fatalf("color %s missing after wrap: %q", color, rows)
		}
	}
}
