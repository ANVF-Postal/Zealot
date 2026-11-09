package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"
)

func TestSplashLogoCyclesOnOpenAndArtworkClick(t *testing.T) {
	m := newMessageInteractionTestModel(110)
	if len(splashLogos) != 7 {
		t.Fatalf("expected original, dollar-sign and five new logos, got %d",len(splashLogos))
	}
	wantRows := []int{7,8,6,7,7,8,7}
	for i,rows := range wantRows {
		if len(splashLogos[i]) != rows {
			t.Fatalf("logo %d has %d rows, want %d",i,len(splashLogos[i]),rows)
		}
	}
	if !strings.HasPrefix(splashLogos[1][0], strings.Repeat("$",8)) ||
		!strings.Contains(splashLogos[1][7], "\\____/") {
		t.Fatal("dollar-sign logo has lost its original artwork")
	}
	for i, prefix := range map[int]string{
		2:"███████╗",
		3:"░▒▓████████▓▒░",
		4:" ________",
		5:" ███████████",
		6:":::::::::",
	} {
		if !strings.HasPrefix(splashLogos[i][0],prefix) {
			t.Fatalf("logo %d changed unexpectedly: %q",i,splashLogos[i][0])
		}
	}

	updated, _ := m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.splashOpen || m.splashLogoIndex == 0 {
		t.Fatalf("opening splash did not choose another logo: open=%v index=%d",m.splashOpen,m.splashLogoIndex)
	}
	if got := stripANSISequences(m.splashRows()[1]); strings.TrimSpace(got)=="" {
		t.Fatal("new splash artwork was not rendered")
	}

	// Clicking a visible position inside the artwork chooses a different logo.
	width := max(1,chatInnerWidth(m.width)-1)
	logoX := 1+splashLogoLeftPadding(m.activeSplashLogo(),width)+1
	if !m.splashLogoContains(logoX,3) {
		t.Fatal("logo artwork does not have a clickable hitbox")
	}
	before := m.splashLogoIndex
	updated, _ = m.Update(tea.MouseClickMsg{X:logoX,Y:3,Button:tea.MouseLeft})
	m = updated.(model)
	if m.splashLogoIndex == before || !m.splashOpen {
		t.Fatalf("clicking splash logo repeated the same design: before=%d after=%d",before,m.splashLogoIndex)
	}

	updated, _ = m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if m.splashOpen { t.Fatal("header click did not close splash") }
	before = m.splashLogoIndex
	updated, _ = m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.splashOpen || m.splashLogoIndex == before {
		t.Fatalf("reopening splash repeated same logo: before=%d after=%d",before,m.splashLogoIndex)
	}
}

func TestSplashLogoClickRespectsScrollAndTerminalWidth(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.splashOpen = true
	for logoIndex := range splashLogos {
		m.splashLogoIndex = logoIndex
		m.splashScroll = 2
		lineIndex := min(3,len(m.activeSplashLogo())-1)
		line := strings.TrimRight(m.activeSplashLogo()[lineIndex]," ")
		y := 3+lineIndex-m.splashScroll
		width := max(1,chatInnerWidth(m.width)-1)
		x := 1+splashLogoLeftPadding(m.activeSplashLogo(),width)+min(2,max(0,lipgloss.Width(line)-1))
		if !m.splashLogoContains(x,y) {
			t.Fatalf("logo %d scrolled row click rejected at (%d,%d)",logoIndex,x,y)
		}
		if m.splashLogoContains(0,y) || m.splashLogoContains(x,2+len(m.splashRows())-1) {
			t.Fatalf("logo %d click area extends outside artwork",logoIndex)
		}
		m.width = 28
		m.splashScroll = 0
		if !m.splashLogoContains(2,3) {
			t.Fatalf("logo %d narrow terminal cannot click visible truncated logo",logoIndex)
		}
		if m.splashLogoContains(28,3) {
			t.Fatalf("logo %d narrow terminal hitbox extends beyond visible logo",logoIndex)
		}
		m.width = 100
	}
}

func TestSplashThemeActionStillWorksAfterSwitchingLogos(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME",t.TempDir())
	m := newThemeEditorTestModel(t)
	m.themesOpen = false
	m.splashOpen = true
	m.height = 36
	before := m.splashLogoIndex
	m.cycleSplashLogo()
	if m.splashLogoIndex == before {
		t.Fatal("splash did not switch to an alternate logo")
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
	before = m.splashLogoIndex
	updated,_ = m.Update(tea.MouseClickMsg{X:1,Y:0,Button:tea.MouseLeft})
	m = updated.(model)
	if m.themesOpen || !m.splashOpen || m.splashLogoIndex == before {
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

func TestSplashLogoArtworkMaintainsFixedBlockAlignment(t *testing.T) {
	for logoIndex, logo := range splashLogos {
		// A trailing 100-space generator pad must never shift the logo.
		const width = 110
		rows := splashLogoLayout(logo,width)
		if len(rows)!=len(logo) {
			t.Fatalf("logo %d row count changed",logoIndex)
		}
		left := splashLogoLeftPadding(logo,width)
		for rowIndex,original := range logo {
			want := strings.Repeat(" ",left)+strings.TrimRight(original," ")
			if rows[rowIndex]!=want {
				t.Fatalf("logo %d row %d shifted or truncated unexpectedly: got %q, want %q",
					logoIndex,rowIndex,rows[rowIndex],want)
			}
			if lipgloss.Width(rows[rowIndex])>width {
				t.Fatalf("logo %d row %d extends past viewport",logoIndex,rowIndex)
			}
		}
		m := newMessageInteractionTestModel(112)
		m.splashOpen = true
		m.splashLogoIndex = logoIndex
		rendered := m.splashRows()
		for i,row := range rows {
			got := stripANSISequences(rendered[i+1])
			if got!=row {
				t.Fatalf("logo %d rendered row %d changed: got %q, want %q",
					logoIndex,i,got,row)
			}
		}
	}
}

func TestSplashLogoClippingRespectsScrollbarAndBlockColumns(t *testing.T) {
	m := newMessageInteractionTestModel(32)
	m.splashOpen = true
	for logoIndex,logo := range splashLogos {
		m.splashLogoIndex = logoIndex
		width := max(1,chatInnerWidth(m.width)-1)
		rows := splashLogoLayout(logo,width)
		for i,row := range rows {
			if lipgloss.Width(row)>width {
				t.Fatalf("logo %d line %d spills into scrollbar",logoIndex,i)
			}
			y := i+3
			if m.splashLogoContains(m.chatScrollbarX(),y) {
				t.Fatalf("logo %d line %d treats scrollbar as clickable artwork",logoIndex,i)
			}
		}
	}
}
