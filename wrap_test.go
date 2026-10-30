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
