package main

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/textarea"
)

func TestInputSelectionCoordinates(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		x          int
		yOffset    int
		wantX      int
		wantY      int
	}{
		{name: "normal width first row", width: 80, x: 4, yOffset: 0, wantX: 0, wantY: 0},
		{name: "normal width wrapped row", width: 80, x: 2, yOffset: 1, wantX: 0, wantY: 1},
		{name: "hidden borders first row", width: 49, x: 3, yOffset: 0, wantX: 0, wantY: 0},
		{name: "hidden borders wrapped row", width: 49, x: 1, yOffset: 1, wantX: 0, wantY: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{width: tt.width, height: 24, input: textarea.New()}
			m.input.SetHeight(2)
			_, top, _, _ := m.inputSelectionArea()
			gotX, gotY := m.inputSelectionCoordinates(tt.x, top+tt.yOffset)
			if gotX != tt.wantX || gotY != tt.wantY {
				t.Fatalf("inputSelectionCoordinates() = (%d, %d), want (%d, %d)", gotX, gotY, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestInputSelectionAreaBounds(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		insideX    int
		outsideX   int
	}{
		{name: "normal borders", width: 80, insideX: 77, outsideX: 78},
		{name: "hidden borders", width: 49, insideX: 47, outsideX: 48},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{width: tt.width, height: 24, input: textarea.New()}
			m.input.SetHeight(2)
			_, top, _, _ := m.inputSelectionArea()
			if !m.inputSelectionContains(tt.insideX, top) {
				t.Fatalf("expected (%d, %d) to be inside composer text area", tt.insideX, top)
			}
			if m.inputSelectionContains(tt.outsideX, top) {
				t.Fatalf("expected (%d, %d) to be outside composer text area", tt.outsideX, top)
			}
		})
	}
}

func TestComposerSelectAllKeyBindings(t *testing.T) {
	m := newModel(defaultConfig(), "lobby", false, nil)
	if got, want := m.input.KeyMap.SelectAll.Keys(), []string{"ctrl+a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectAll keys = %v, want %v", got, want)
	}
	if got, want := m.input.KeyMap.LineStart.Keys(), []string{"home"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LineStart keys = %v, want %v", got, want)
	}
}
