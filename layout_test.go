package main

import "testing"

func TestNarrowBorderThreshold(t *testing.T) {
	tests := []struct {
		width int
		want  bool
	}{
		{width: 51, want: false},
		{width: 50, want: false},
		{width: 49, want: true},
		{width: 48, want: true},
		{width: 30, want: true},
	}

	for _, tt := range tests {
		if got := narrowBordersHidden(tt.width); got != tt.want {
			t.Errorf("narrowBordersHidden(%d) = %v, want %v", tt.width, got, tt.want)
		}
	}
}

func TestChatContentWidthWithHiddenBorders(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{width: 51, want: 48},
		{width: 50, want: 47},
		{width: 49, want: 48},
		{width: 48, want: 47},
		{width: 30, want: 29},
	}

	for _, tt := range tests {
		if got := chatContentWidth(tt.width); got != tt.want {
			t.Errorf("chatContentWidth(%d) = %d, want %d", tt.width, got, tt.want)
		}
	}
}
