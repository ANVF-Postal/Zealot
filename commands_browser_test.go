package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestHandbookPublicCategoriesAndSplashButton(t *testing.T) {
	if len(handbookPages) != 4 ||
		handbookPages[0].Name != "CHAT" ||
        handbookPages[1].Name != "VISUAL" ||
		handbookPages[2].Name != "USER" ||
		handbookPages[3].Name != "LAUNCH" {
		t.Fatalf("incorrect handbook categories: %+v", handbookPages)
	}
	// Keep the original public entries covered while allowing new commands
	// to be added without failing this test for a legitimate feature expansion.
	if len(handbookPages[0].Entries) < 16 ||
        len(handbookPages[1].Entries) < 17 ||
		len(handbookPages[2].Entries) < 4 ||
		len(handbookPages[3].Entries) < 2 {
		t.Fatal("handbook is missing public commands or click behaviors")
	}
	chatCommands := make(map[string]bool, len(handbookPages[0].Entries))
	for _, entry := range handbookPages[0].Entries {
		chatCommands[entry.Command] = true
	}
    visualCommands:=make(map[string]bool,len(handbookPages[1].Entries))
    for _,entry:=range handbookPages[1].Entries {
        visualCommands[entry.Command]=true
        if strings.Contains(entry.Command," self [seconds]") {
            t.Fatalf("duplicate self-only visual command %q",entry.Command)
        }
        if chatCommands[entry.Command] {
            t.Fatalf("visual command %q also appears on CHAT tab",entry.Command)
        }
    }
    for _,command:=range []string{
        "/rain [seconds]","/snow [seconds]","/blizzard [seconds]",
        "/embers [seconds]","/fire [seconds]","/matrix [seconds]",
        "/static [seconds]","/glitch [seconds]","/interrogation [seconds]",
        "/bubbles [seconds]","/aquarium [seconds]","/terminal [seconds]",
        "/fireworks [seconds]","/confetti [seconds]",
        "/terminal off","/stop","/stopfx self","/disablefx","/fxfps 1-60",
    } {
        if !visualCommands[command] {
            t.Fatalf("VISUAL tab missing command %q",command)
        }
    }
    if !chatCommands["/rainbow"] || !chatCommands["/rb"] {
        t.Fatal("rainbow chat commands should remain on CHAT tab")
    }
	m := newMessageInteractionTestModel(100)
	m.splashOpen = true
	rows := m.splashRows()
	commands, themes := -1,-1
	for i,row := range rows {
		text := stripANSISequences(row)
		if strings.Contains(text,"[COMMANDS]") { commands=i }
		if strings.Contains(text,"[THEMES]") { themes=i }
	}
	if commands < 0 || themes != commands+1 { t.Fatalf("splash buttons not stacked correctly: %d and %d",commands,themes) }
	x := 1+(chatContentWidth(m.width)-lipgloss.Width("[COMMANDS]"))/2+2
	y := 2+commands-m.splashScroll
	if !m.splashCommandsButtonContains(x,y) { t.Fatal("Commands button hitbox missing") }
	updated,_:=m.Update(tea.MouseClickMsg{Button:tea.MouseLeft,X:x,Y:y})
	m=updated.(model)
	if !m.commandsOpen || !m.splashOpen || m.commandsTab!=0 { t.Fatal("click failed to open handbook") }
	if m.themesOpen { t.Fatal("handbook accidentally opened themes") }
	updated,_=m.Update(tea.MouseClickMsg{Button:tea.MouseLeft,X:m.width-2,Y:0})
	m=updated.(model)
	if m.commandsOpen || !m.splashOpen { t.Fatal("header close should return to splash") }
}

func TestHandbookFolderTabClickAndEscape(t *testing.T) {
	m:=newMessageInteractionTestModel(90)
	m.openCommandsBrowser()
	rim,face,regions:=m.commandsTabsLine()
	if !strings.Contains(stripANSISequences(rim),"╭") ||
		!strings.Contains(stripANSISequences(face),"│ CHAT │") {
		t.Fatalf("folder tab artwork missing: %q %q",rim,face)
	}
	if len(regions)!=4 { t.Fatal("expected four click regions") }
	for _,tab := range []int{1,2,3,0} {
		x:=1+regions[tab].Start+1
		updated,_:=m.Update(tea.MouseClickMsg{Button:tea.MouseLeft,X:x,Y:3})
		m=updated.(model)
		if m.commandsTab!=tab || m.commandsScroll!=0 {
			t.Fatalf("tab click did not switch/reset: got %d scroll %d, wanted %d",m.commandsTab,m.commandsScroll,tab)
		}
		_,_,regions=m.commandsTabsLine()
	}
	updated,_:=m.Update(tea.KeyPressMsg{Code:27})
	m=updated.(model)
	if m.commandsOpen || !m.splashOpen { t.Fatal("Escape did not return from handbook to splash") }
}

func TestHandbookVisualIntroductionAndCompactNavigation(t *testing.T) {
    for _,width:=range []int{19,23,45,100} {
        m:=newMessageInteractionTestModel(width)
        m.openCommandsBrowser()
        m.selectCommandsTab(1)
        rows:=m.commandsTextRows()
        if len(rows)<3 {
            t.Fatalf("width %d: VISUAL page empty",width)
        }
        if intro:=strings.Join(func() []string {
            lines:=make([]string,0)
            for _,row:=range rows {
                if row=="" {break}
                lines=append(lines,stripANSISequences(row))
            }
            return lines
        }()," ");!strings.Contains(strings.Join(strings.Fields(intro)," "),handbookVisualIntro) {
            t.Fatalf("width %d: missing VISUAL intro: %q",width,intro)
        }
        rendered:=m.renderCommandsBrowser()
        if !strings.Contains(stripANSISequences(rendered),"VISUAL") && width>=45 {
            t.Fatalf("width %d: VISUAL folder not shown",width)
        }
        for _,row:=range rows {
            if lipgloss.Width(row)>chatContentWidth(width) {
                t.Fatalf("width %d: VISUAL row overflow: %q",width,row)
            }
        }
        _,_,hits:=m.commandsTabsLine()
        if len(hits)!=4 {t.Fatalf("width %d: expected 4 folder hitboxes",width)}
        if hits[1].End<=hits[1].Start {
            t.Fatalf("width %d: selected VISUAL folder is inaccessible",width)
        }
    }
}

func TestHandbookWordWrapAndScroll(t *testing.T) {
	m:=newMessageInteractionTestModel(45)
	m.openCommandsBrowser()
	rows:=m.commandsTextRows()
	if len(rows)<=m.commandsVisibleHeight() { t.Fatal("test needs scrollable contents") }
	for _,row:=range rows {
		if lipgloss.Width(row)>chatContentWidth(m.width) {
			t.Fatalf("handbook text overflows its chat column: width=%d row=%q",lipgloss.Width(row),row)
		}
	}
	updated,_:=m.Update(tea.MouseWheelMsg{Button:tea.MouseWheelDown})
	m=updated.(model)
	if m.commandsScroll!=3 { t.Fatalf("wheel did not scroll handbook: %d",m.commandsScroll) }
	_,targets:=m.renderStatusBar(false,false)
	if !targets.hasSend || !targets.hasPageScroll { t.Fatal("handbook footer lost Send or paging actions") }
	before:=m.commandsScroll
	updated,_=m.Update(tea.MouseClickMsg{Button:tea.MouseLeft,X:targets.pgDnStart,Y:m.height-1})
	m=updated.(model)
	if m.commandsScroll<=before { t.Fatal("footer PgDn did not scroll handbook") }
	m.selectCommandsTab(3)
	if m.commandsScroll!=0 { t.Fatal("changing tabs did not reset scroll") }
}

func TestHandbookScrollbarDragAndRelease(t *testing.T) {
	m:=newMessageInteractionTestModel(37)
	m.openCommandsBrowser()
	if m.maxCommandsScroll()==0 { t.Fatal("test needs scrollable page") }
	x:=m.chatScrollbarX()
	_,thumb,track:=m.commandsScrollbarThumb()
	clickY:=3+min(track-1,thumb+1)
	updated,_:=m.Update(tea.MouseClickMsg{Button:tea.MouseLeft,X:x,Y:clickY})
	m=updated.(model)
	if m.commandsScroll==0 { t.Fatal("scroll track click did not move") }
	updated,_=m.Update(tea.MouseReleaseMsg{})
	m=updated.(model)
	if m.commandsDragging || m.commandsHoldDirection!=0 { t.Fatal("mouse release did not stop scrollbar interactions") }
}

func TestHandbookCompactTabsOverlayAndNavigation(t *testing.T) {
	m := newMessageInteractionTestModel(23)
	m.openCommandsBrowser()
	if !m.commandsTabsOverflow() {
		t.Fatal("compact window should show tab navigation overlay")
	}
	rim, face, hits := m.commandsTabsLine()
	if !strings.Contains(stripANSISequences(face), "< | >") {
		t.Fatalf("compact tabs missing arrow overlay: %q", face)
	}
	if got := lipgloss.Width(face); got != chatContentWidth(m.width) {
		t.Fatalf("compact tab face width = %d, content width = %d", got, chatContentWidth(m.width))
	}
	if got := lipgloss.Width(rim); got != chatContentWidth(m.width) {
		t.Fatalf("compact tab rim width = %d, content width = %d", got, chatContentWidth(m.width))
	}
	bg := lipgloss.NewStyle().Foreground(lipgloss.Color(m.cfg.Theme.AccentColor)).
		Background(lipgloss.Color(m.cfg.Theme.Background)).Bold(true)
	if !strings.Contains(face, bg.Render("< | >")) {
		t.Fatalf("navigation must use persistent theme background and accent foreground: %q", face)
	}
	for _, hit := range hits {
		if hit.End > chatContentWidth(m.width)-lipgloss.Width(commandsTabNavigation) {
			t.Fatalf("tab hitbox leaks under arrow overlay: %+v", hit)
		}
	}

	left := 0
	if !narrowBordersHidden(m.width) { left = 1 }
	arrowX := left + chatContentWidth(m.width)-1
	for _, target := range []int{1, 2, 3} {
		updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: arrowX, Y: 3})
		m = updated.(model)
		if m.commandsTab != target || m.commandsScroll != 0 {
			t.Fatalf("right arrow failed to advance to %d, got tab=%d scroll=%d", target, m.commandsTab, m.commandsScroll)
		}
		_, visible, regions := m.commandsTabsLine()
		if regions[target].End <= regions[target].Start {
			t.Fatalf("selected tab %d is invisible in compact layout: %q", target, visible)
		}
	}
	// Endpoints stop at the final tab, rather than wrapping to CHAT.
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: arrowX, Y: 3})
	m = updated.(model)
	if m.commandsTab != 3 { t.Fatal("right arrow wrapped beyond LAUNCH") }

	leftArrowX := left + chatContentWidth(m.width)-lipgloss.Width(commandsTabNavigation)
	for _, target := range []int{2, 1, 0} {
		updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: leftArrowX, Y: 2})
		m = updated.(model)
		if m.commandsTab != target { t.Fatalf("left arrow failed to select %d; got %d", target, m.commandsTab) }
	}
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: leftArrowX, Y: 3})
	if got := updated.(model).commandsTab; got != 0 { t.Fatalf("left arrow wrapped to tab %d", got) }
}

func TestHandbookTabNavigationDisappearsWhenThereIsRoom(t *testing.T) {
	m := newMessageInteractionTestModel(100)
	m.openCommandsBrowser()
	if m.commandsTabsOverflow() { t.Fatal("wide window incorrectly shows navigation") }
	_, face, regions := m.commandsTabsLine()
	if strings.Contains(stripANSISequences(face), commandsTabNavigation) {
		t.Fatal("unnecessary overlay appeared in wide layout")
	}
	for i, region := range regions {
		if region.End <= region.Start {
			t.Fatalf("wide layout hid tab %d", i)
		}
	}

	// Resizing while on the LAUNCH tab must keep that tab reachable.
	m.selectCommandsTab(3)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 24})
	m = updated.(model)
	if m.commandsTab != 3 || !m.commandsTabsOverflow() {
		t.Fatalf("resize lost selected tab or overflow: tab=%d", m.commandsTab)
	}
	_, face, regions = m.commandsTabsLine()
	if regions[3].End <= regions[3].Start {
		t.Fatalf("LAUNCH tab not kept visible after resizing: %q", face)
	}
}


func TestHandbookRowsCachedAcrossScrollAndInvalidatedOnChanges(t *testing.T) {
	m := newMessageInteractionTestModel(45)
	m.openCommandsBrowser()
	if len(m.commandsRowsCache) == 0 { t.Fatal("handbook did not prebuild its formatted rows") }
	first := &m.commandsRowsCache[0]
	if got := &m.commandsTextRows()[0]; got != first {
		t.Fatal("handbook regenerated rows instead of using the cached layout")
	}
	for i := 0; i < 80; i++ {
		updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		m = updated.(model)
		if got := &m.commandsTextRows()[0]; got != first {
			t.Fatalf("scroll event %d rebuilt unchanged handbook text", i)
		}
	}
	m.selectCommandsTab(1)
	if got := &m.commandsTextRows()[0]; got == first {
		t.Fatal("switching tabs did not invalidate the cache")
	}
	if m.commandsRowsCacheTab != 1 { t.Fatal("cached tab was not updated") }
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 34, Height: m.height})
	m = updated.(model)
	if m.commandsRowsCacheWidth != 34 { t.Fatal("resizing did not regenerate wrapped rows") }
	m.cfg.Theme.AccentColor = "#334455"
	if m.commandsRowsCacheTheme == m.cfg.Theme { t.Fatal("test did not change the theme") }
	if got := m.commandsTextRows(); len(got) == 0 {
		t.Fatal("theme change resulted in empty handbook")
	}
	m.closeCommandsBrowser()
	if m.commandsOpen { t.Fatal("handbook failed to close after scrolling") }
}

func BenchmarkHandbookScrollAndRender(b *testing.B) {
	m := newMessageInteractionTestModel(60)
	m.openCommandsBrowser()
	b.ResetTimer()
	for i:=0; i<b.N; i++ {
		m.commandsScroll = i % (m.maxCommandsScroll()+1)
		_ = m.renderCommandsBrowser()
	}
}
