package main

// inputSelectionArea returns the screen-space bounds of the textarea's visible
// content, excluding the composer border and horizontal padding.
func (m model) inputSelectionArea() (left, top, right, bottom int) {
	left = 1 // composer left padding
	right = m.width - 1 // composer right padding
	if !narrowBordersHidden(m.width) {
		left++
		right--
	}
	top = 4 + m.messageHeight() // header, chat pane, composer border, and content inset
	bottom = top + m.input.Height()
	return
}

// inputSelectionCoordinates converts screen coordinates into the coordinate
// space expected by bubbles/textarea. The first row has Zealot's "> " prompt
// rendered outside the textarea; wrapped rows do not.
func (m model) inputSelectionCoordinates(x, y int) (int, int) {
	left, top, _, _ := m.inputSelectionArea()
	localY := y - top
	localX := x - left
	if localY == 0 {
		localX -= 2
		if m.replyTargetID != "" {
			localX -= len([]rune(replyComposerPrefix))
		}
	}
	return localX, localY
}

func (m model) inputSelectionContains(x, y int) bool {
	left, top, right, bottom := m.inputSelectionArea()
	return x >= left && x < right && y >= top && y < bottom
}
