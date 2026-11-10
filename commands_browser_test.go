package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestHandbookPublicCategoriesAndSplashButton(t *testing.T) {
	if len(handbookPages) != 3 ||
		handbookPages[0].Name != "CHAT" ||
		handbookPages[1].Name != "USER" ||
		handbookPages[2].Name != "LAUNCH" {
		t.Fatalf("incorrect handbook categories: %+v", handbookPages)
	}
	if len(handbookPages[0].Entries) != 10 ||
		len(handbookPages[1].Entries) != 4 ||
		len(handbookPages[2].Entries) != 2 {
		t.Fatal("handbook is missing public commands or click behaviors")
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
	if len(regions)!=3 { t.Fatal("expected three click regions") }
	for _,tab := range []int{1,2,0} {
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
	m.selectCommandsTab(2)
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
