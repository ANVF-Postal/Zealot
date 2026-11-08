package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestThemeWheelMovesOneEntryAndScrollsOnlyHoveredPane(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.height = 16
	m.clampThemeSelection()
	if !m.themeListOverflow() {
		t.Fatal("fixture needs more themes than visible rows")
	}
	if !m.themeDetailsOverflow() {
		t.Fatal("short viewport should require details scrolling")
	}
	m.themeSelected = 0
	m.themeScroll = 0
	m.themeDetailsScroll = 0

	listX := m.themeBrowserContentStartX()+1
	updated, _ := m.Update(tea.MouseWheelMsg{X:listX,Y:5,Button:tea.MouseWheelDown})
	m = updated.(model)
	if m.themeSelected != 1 || m.themeDetailsScroll != 0 {
		t.Fatalf("list wheel should move exactly one item, got selected=%d detailsScroll=%d",
			m.themeSelected,m.themeDetailsScroll)
	}
	_, rightWidth := m.themeBrowserColumnWidths()
	rightX := m.themeBrowserDividerX()+max(1,rightWidth/2)
	updated, _ = m.Update(tea.MouseWheelMsg{X:rightX,Y:6,Button:tea.MouseWheelDown})
	m = updated.(model)
	if m.themeSelected != 1 || m.themeDetailsScroll != 1 {
		t.Fatalf("details wheel must scroll independently by one: selected=%d detailsScroll=%d",
			m.themeSelected,m.themeDetailsScroll)
	}
	updated, _ = m.Update(tea.MouseWheelMsg{X:rightX,Y:6,Button:tea.MouseWheelUp})
	m = updated.(model)
	if m.themeDetailsScroll != 0 || m.themeSelected != 1 {
		t.Fatal("scrolling details upward changed the selected theme")
	}
}

func TestThemePaneScrollbarsTrackOverflowAndUseSeparateHitboxes(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.height = 14
	if !m.themeListOverflow() || !m.themeDetailsOverflow() {
		t.Fatal("both panes should overflow at this height")
	}
	leftWidth, rightWidth := m.themeBrowserColumnWidths()
	listBarX := m.themeBrowserContentStartX()+leftWidth-1
	detailBarX := m.themeBrowserDividerX()+rightWidth
	if !m.themeListScrollbarContains(listBarX,5) ||
		!m.themeDetailsScrollbarContains(detailBarX,5) {
		t.Fatal("scrollbar hitboxes do not match their positions")
	}
	if _, ok := m.themeBrowserListIndexAt(listBarX,5); ok {
		t.Fatal("list scrollbar was treated as a theme row")
	}
	if field, ok := m.themeColorFieldAt(detailBarX,5); ok {
		t.Fatalf("details scrollbar was treated as color edit %q",field)
	}
	panel := stripANSISequences(m.renderThemeBrowser())
	if !strings.Contains(panel,"█") || !strings.Contains(panel,"│") {
		t.Fatalf("scrollbar track/thumbnail not visible: %q",panel)
	}
	selected := m.themeSelected
	updated, _ := m.Update(tea.MouseClickMsg{X:listBarX,Y:4+m.themeVisibleRows()-1,Button:tea.MouseLeft})
	m = updated.(model)
	if m.themeScroll <= 0 || m.themeSelected != selected {
		t.Fatalf("list arrow should scroll viewport without changing selection: offset=%d selected=%d",m.themeScroll,m.themeSelected)
	}
	updated, _ = m.Update(tea.MouseClickMsg{X:detailBarX,Y:2+m.messageHeight()-1,Button:tea.MouseLeft})
	m = updated.(model)
	if m.themeDetailsScroll != 1 {
		t.Fatalf("details bottom arrow should move one line: scroll=%d",m.themeDetailsScroll)
	}
}

func TestThemeDetailsScrollMakesDescriptionAndActionsReachable(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.cloneSelectedTheme()
	m.height = 14
	_, rightWidth := m.themeBrowserColumnWidths()
	if !m.themeDetailsOverflow() {
		t.Fatal("details should overflow on short terminal")
	}
	m.themeDetailsScroll = m.maxThemeDetailsScroll()
	panel := stripANSISequences(m.renderThemeBrowser())
	if !strings.Contains(panel,"[CLONE]") || !strings.Contains(panel,"[APPLY]") {
		t.Fatalf("scrolled details did not show action row: %q",panel)
	}
	detailWidth := rightWidth-1
	row := m.themeActionButtonRowForWidth(detailWidth)
	y := 2+row-m.themeDetailsScroll
	start,end,ok := themeActionButtonBounds(detailWidth,themeCloneButtonLabel,false)
	if !ok {
		t.Fatal("missing Clone action layout")
	}
	x := m.themeBrowserDividerX()+1+(start+end)/2
	if !m.themeCloneButtonContains(x,y) {
		t.Fatalf("Clone hitbox not translated by scroll: x=%d y=%d",x,y)
	}
	// The color edit action must also map from visible rows to the
	// correct underlying palette entry after scrolling.
	m.themeDetailsScroll = min(2,m.maxThemeDetailsScroll())
	colorY := 2+themePaletteFirstRow+3-m.themeDetailsScroll
	field,ok := m.themeColorFieldAt(m.themeBrowserDividerX()+3,colorY)
	if !ok || field!="accent_color" {
		t.Fatalf("scrolled color row hitbox = %q,%v",field,ok)
	}
	m.themeDetailsScroll = m.maxThemeDetailsScroll()
	m.moveThemeSelection(1)
	if m.themeDetailsScroll!=0 {
		t.Fatal("changing theme did not reset the details scroll")
	}
}

func TestThemeDetailsDoNotShowScrollbarWhenRoomy(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.height = 70
	if m.themeDetailsOverflow() {
		t.Fatal("details should fit on a tall terminal")
	}
	if m.themeListOverflow() {
		t.Fatal("theme list should fit on a tall terminal")
	}
	_,rightWidth := m.themeBrowserColumnWidths()
	if !m.themeDetailsScrollbarContains(m.themeBrowserDividerX()+rightWidth,5) {
		t.Fatal("chat-style scrollbar rail should remain visible without overflow")
	}
	if m.themeScrollbarGlyph(themeDetailsPane,0)!="^" ||
		m.themeScrollbarGlyph(themeDetailsPane,m.messageHeight()-1)!="v" {
		t.Fatal("details rail is missing chat-style arrow endpoints")
	}
	m.scrollThemeDetails(1000)
	if m.themeDetailsScroll!=0 {
		t.Fatal("details scroll offset escaped bounds without overflow")
	}
}

func TestThemePackButtonWorksWithDetailsScrollbar(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.cloneSelectedTheme()
	m.height = 14
	if !m.themeDetailsOverflow() {
		t.Fatal("short terminal should show details scrollbar")
	}
	_, width := m.themeBrowserColumnWidths()
	barX := m.themeBrowserDividerX()+width
	if m.themePackButtonContains(barX,2) {
		t.Fatal("scrollbar column must not be mistaken for Pack")
	}
	x := barX-len(themePackButtonLabel)
	if !m.themePackButtonContains(x,2) {
		t.Fatalf("Pack hitbox missing beside details scrollbar at x=%d",x)
	}
	updated,_ := m.Update(tea.MouseClickMsg{X:x,Y:2,Button:tea.MouseLeft})
	m = updated.(model)
	if !m.themePacking || len(m.themePackSelected)!=1 {
		t.Fatal("Pack button failed in narrow-height layout")
	}
}

func TestThemeDetailsLongDescriptionCanScrollToLastRow(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.cloneSelectedTheme()
	entry,ok := m.selectedThemeEntry()
	if !ok {t.Fatal("missing custom theme")}
	entry.Preset.Description = strings.Repeat("The long description should wrap across the details viewport. ",20)
	m.themeEntries[m.themeSelected] = entry
	m.height = 16
	if !m.themeDetailsOverflow() {t.Fatal("long description did not overflow")}
	m.themeDetailsScroll = m.maxThemeDetailsScroll()
	_,rightWidth := m.themeBrowserColumnWidths()
	width := rightWidth-1
	actionRow := m.themeActionButtonRowForWidth(width)
	y := 2+actionRow-m.themeDetailsScroll
	if y < 2 || y >= 2+m.messageHeight() {
		t.Fatalf("last action row is still inaccessible: y=%d",y)
	}
	start,end,ok := themeActionButtonBounds(width,themeApplyButtonLabel,false)
	if !ok {t.Fatal("missing Apply action")}
	x := m.themeBrowserDividerX()+1+(start+end)/2
	if !m.themeApplyButtonContains(x,y) {
		t.Fatalf("Apply button hitbox is not reachable after scrolling to end at (%d,%d)",x,y)
	}
}

func TestThemeScrollbarsDragThumbLikeChat(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.height = 14
	for _, pane := range []themeScrollPane{themeListPane,themeDetailsPane} {
		top,thumb,track := m.themeScrollbarGeometry(pane)
		if track <= thumb {t.Fatalf("pane %d has no draggable range",pane)}
		x := m.themeBrowserContentStartX()
		topY := 4
		if pane == themeListPane {
			left,_ := m.themeBrowserColumnWidths()
			x += left-1
		} else {
			_,right := m.themeBrowserColumnWidths()
			x = m.themeBrowserDividerX()+right
			topY = 2
		}
		// Mouse down on the visible thumb, drag to the bottom and release.
		updated,_ := m.Update(tea.MouseClickMsg{X:x,Y:topY+1+top,Button:tea.MouseLeft})
		m = updated.(model)
		if pane==themeListPane && !m.themeListScrollDragging ||
			pane==themeDetailsPane && !m.themeDetailsScrollDragging {
			t.Fatalf("pane %d did not grab its thumb",pane)
		}
		m.applyMouseMotion(x,topY+track)
		visible,total,_ := m.themeScrollbarDimensions(pane)
		end := max(0,total-visible)
		offset := m.themeScroll
		if pane==themeDetailsPane {offset=m.themeDetailsScroll}
		if offset!=end {
			t.Fatalf("pane %d drag did not reach bottom: offset=%d want=%d",pane,offset,end)
		}
		updated,_ = m.Update(tea.MouseReleaseMsg{X:x,Y:topY+track,Button:tea.MouseLeft})
		m = updated.(model)
		if m.themeListScrollDragging || m.themeDetailsScrollDragging {
			t.Fatal("thumb drag remained active after release")
		}
		// Reset to the top before testing the other pane.
		m.setThemeScrollFromThumbTop(pane,0)
	}
}

func TestThemeScrollbarArrowHoldRepeatsAndReleaseStops(t *testing.T) {
	m := newThemeEditorTestModel(t)
	m.height = 14
	left,_ := m.themeBrowserColumnWidths()
	x := m.themeBrowserContentStartX()+left-1
	bottom := 4+m.themeVisibleRows()-1
	updated,cmd := m.Update(tea.MouseClickMsg{X:x,Y:bottom,Button:tea.MouseLeft})
	m = updated.(model)
	if cmd==nil || m.themeListScroll<=0 || m.themeListScrollbarHoldDirection!=1 {
		t.Fatalf("bottom arrow did not initiate scroll and hold: offset=%d hold=%d",m.themeScroll,m.themeListScrollbarHoldDirection)
	}
	generation := m.themeListScrollbarHoldGeneration
	before := m.themeScroll
	updated,_ = m.Update(themeScrollbarHoldMsg{pane:themeListPane,generation:generation})
	m = updated.(model)
	if m.themeScroll<=before {
		t.Fatal("held scrollbar arrow did not repeat")
	}
	updated,_ = m.Update(tea.MouseReleaseMsg{X:x,Y:bottom,Button:tea.MouseLeft})
	m = updated.(model)
	if m.themeListScrollbarHoldDirection!=0 {
		t.Fatal("release did not stop the scrollbar arrow")
	}
	before=m.themeScroll
	updated,_ = m.Update(themeScrollbarHoldMsg{pane:themeListPane,generation:generation})
	m = updated.(model)
	if m.themeScroll!=before {
		t.Fatal("stale hold message scrolled after release")
	}
}
