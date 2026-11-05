package main

import (
	"strings"
	"testing"
)

func TestWrapTextPrefersSpacesOverHyphenatingNextWord(t *testing.T) {
	got := wrapText("hello world", 8)
	want := []string{"hello", "world"}
	if len(got) != len(want) {
		t.Fatalf("wrapText() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wrapText() = %#v, want %#v", got, want)
		}
	}
}

func TestWrapTextHyphenatesOversizedUnbrokenWord(t *testing.T) {
	got := wrapText("abcdefghijk", 8)
	want := []string{"abcdefg-", "hijk"}
	if len(got) != len(want) {
		t.Fatalf("wrapText() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wrapText() = %#v, want %#v", got, want)
		}
	}
}

func TestWrapTextURLBreaksDoNotAddCharacters(t *testing.T) {
	url := "https://example.com/path-with-a-verylongsegment"
	got := wrapText(url, 12)
	if joined := strings.Join(got, ""); joined != url {
		t.Fatalf("wrapped URL changed: joined lines = %q, want %q (lines %#v)", joined, url, got)
	}
	for _, line := range got {
		if len(line) > 12 {
			t.Fatalf("wrapped URL line exceeds width: %q", line)
		}
	}
}

func TestWrapTextLineCountMatchesRenderedRows(t *testing.T) {
	cases := []struct {
		text string
		first, second, full int
	}{
		{"", 8, 8, 8},
		{"hello world", 8, 8, 8},
		{"abcdefghijk", 8, 8, 8},
		{"https://example.com/path-with-a-verylongsegment", 12, 12, 12},
		{"one two three four five", 3, 7, 10},
		{"first line\n\nthird line", 12, 8, 12},
		{"a\tb c", 1, 2, 3},
		{strings.Repeat("界", 17) + " tail", 5, 8, 10},
		{"e\u0301e\u0301e\u0301 tail", 2, 2, 2},
		{"👨‍👩‍👧‍👦👨‍👩‍👧‍👦 tail", 2, 2, 2},
		{"🇳🇱🇳🇱🇳🇱 tail", 2, 2, 2},
	}
	for _, tc := range cases {
		got := wrapTextLineCountVariable(tc.text, tc.first, tc.second, tc.full)
		want := len(wrapTextVariable(tc.text, tc.first, tc.second, tc.full))
		if got != want {
			t.Errorf("line count for %q at widths %d/%d/%d = %d, want %d",
				tc.text, tc.first, tc.second, tc.full, got, want)
		}
	}
}


func TestSplitLongWordPrefixKeepsGraphemeClustersIntact(t *testing.T) {
	cases := []struct {
		word  string
		width int
	}{
		{"e\u0301e\u0301", 1},
		{"👨‍👩‍👧‍👦X", 2},
		{"🇳🇱🇺🇸", 2},
	}
	for _, tc := range cases {
		part, rest, _ := splitLongWordPrefix(tc.word, tc.width, false)
		if part == "" {
			t.Errorf("splitLongWordPrefix(%q, %d) returned an empty prefix", tc.word, tc.width)
			continue
		}
		if rest != "" && strings.HasPrefix(rest, "\u0301") {
			t.Errorf("splitLongWordPrefix(%q, %d) split off a combining mark: part=%q rest=%q", tc.word, tc.width, part, rest)
		}
		if strings.Contains(part, "\u200d") && !strings.Contains(part, "👨‍👩‍👧‍👦") {
			t.Errorf("splitLongWordPrefix(%q, %d) split a joined emoji: part=%q rest=%q", tc.word, tc.width, part, rest)
		}
	}
}
