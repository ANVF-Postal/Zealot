package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSplashLogoCyclesOnOpenAndArtworkClick(t *testing.T) {
	m := newMessageInteractionTestModel(110)
	if len(splashLogos) != 2 {
		t.Fatalf("expected original and new logo, got %d", len(splashLogos))
	}
	if len(splashLogos[0]) != 7 || len(splashLogos[1]) != 8 {
		t.Fatal("splash logos do not preserve their original row counts")
	}
	if !strings.HasPrefix(splashLogos[1][0], strings.Repeat("$",8)) ||
		!strings.Contains(splashLogos[1][7], "\\____/") {
		t.Fatal("new dollar-sign logo has lost its original artwork")
	}

	// Clicking ZEALOT in the top bar opens the splash and picks a new logo.
	updated, _ := m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.splashOpen || m.splashLogoIndex != 1 {
		t.Fatalf("opening splash did not change logo: open=%v index=%d", m.splashOpen,m.splashLogoIndex)
	}
	rows := m.splashRows()
	if !strings.Contains(stripANSISequences(rows[1]), strings.Repeat("$",8)) {
		t.Fatal("new splash artwork was not rendered")
	}

	firstLine := m.activeSplashLogo()[0]
	width := chatContentWidth(m.width)
	logoX := 1 + (width-len(firstLine))/2 + 1
	if !m.splashLogoContains(logoX,3) {
		t.Fatal("logo artwork does not have a clickable hitbox")
	}
	updated, _ = m.Update(tea.MouseClickMsg{X:logoX,Y:3,Button:tea.MouseLeft})
	m = updated.(model)
	if m.splashLogoIndex != 0 || !m.splashOpen {
		t.Fatalf("clicking splash logo did not cycle: index=%d open=%v",m.splashLogoIndex,m.splashOpen)
	}

	// Each new opening should choose another logo as well.
	updated, _ = m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if m.splashOpen {
		t.Fatal("header click did not close splash")
	}
	updated, _ = m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.splashOpen || m.splashLogoIndex != 1 {
		t.Fatalf("reopening splash failed to rotate: %+v",m.splashLogoIndex)
	}
}

func TestSplashLogoClickRespectsScrollAndTerminalWidth(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.splashOpen = true
	m.cycleSplashLogo()
	m.splashScroll = 2
	lineIndex := 3
	line := m.activeSplashLogo()[lineIndex]
	y := 3+lineIndex-m.splashScroll
	x := 1 + (chatContentWidth(m.width)-len(line))/2 + 2
	if !m.splashLogoContains(x,y) {
		t.Fatalf("scrolled logo row click rejected at (%d,%d)",x,y)
	}
	if m.splashLogoContains(0,y) || m.splashLogoContains(x,2+len(m.splashRows())-1) {
		t.Fatal("logo click area extends outside the artwork")
	}
	m.width = 28
	m.splashScroll = 0
	if !m.splashLogoContains(2,3) {
		t.Fatal("narrow terminal cannot click visible truncated logo")
	}
	if m.splashLogoContains(28,3) {
		t.Fatal("narrow terminal hitbox extends beyond visible logo")
	}
}

func TestSplashThemeActionStillWorksAfterSwitchingLogos(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME",t.TempDir())
	m := newThemeEditorTestModel(t)
	m.themesOpen = false
	m.splashOpen = true
	m.height = 36
	m.cycleSplashLogo()
	if m.splashLogoIndex != 1 {
		t.Fatal("splash did not switch to the additional logo")
	}
	button := "[THEMES]"
	x := 1 + max(0,(chatContentWidth(m.width)-len(button))/2)
	y := 2+len(m.splashRows())-1
	if !m.splashThemesButtonContains(x,y) {
		t.Fatal("[THEMES] button moved or lost its hitbox")
	}
	updated,_ := m.Update(tea.MouseClickMsg{X:x,Y:y,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.themesOpen {
		t.Fatal("clicking [THEMES] no longer opens the theme browser")
	}
	updated,_ = m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if m.themesOpen || !m.splashOpen || m.splashLogoIndex != 0 {
		t.Fatalf("returning to splash failed to rotate logo: themes=%v splash=%v index=%d",
			m.themesOpen,m.splashOpen,m.splashLogoIndex)
	}
}

func TestEscapeFromThemeBrowserRotatesSplashArtwork(t *testing.T) {
	m := newThemeEditorTestModel(t)
	if !m.themesOpen || !m.splashOpen {
		t.Fatal("theme editor fixture should be opened from splash")
	}
	before := m.splashLogoIndex
	updated,_ := m.Update(tea.KeyPressMsg{Code:tea.KeyEscape})
	m = updated.(model)
	if m.themesOpen || !m.splashOpen {
		t.Fatal("escape did not return from theme browser to splash")
	}
	if m.splashLogoIndex == before {
		t.Fatal("returning to splash did not choose another logo")
	}
}
