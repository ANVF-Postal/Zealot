package main

// scrollbarGeometry is shared by the chat history and long-message reader.
// scrollFromTop is a top-origin row offset; the main chat converts its
// bottom-origin scroll before calling.
func scrollbarGeometry(total, visible, trackHeight, scrollFromTop int) (top, thumbHeight, track int) {
	track = max(1, trackHeight)
	visible = max(1, visible)
	if total <= visible {
		return 0, track, track
	}
	thumbHeight = max(1, min(track, visible*track/total))
	maxTop := track - thumbHeight
	maxScroll := total - visible
	top = scaleScrollbarPosition(min(max(scrollFromTop, 0), maxScroll), maxScroll, maxTop)
	return
}

// scrollbarScrollForThumb maps a dragged thumb to a top-origin row offset.
// This is the inverse of scrollbarGeometry and uses the same rounded
// positioning math as the main chat scrollbar.
func scrollbarScrollForThumb(top, total, visible, trackHeight, thumbHeight int) int {
	maxScroll := max(0, total-max(1, visible))
	maxTop := max(0, trackHeight-thumbHeight)
	if maxTop == 0 {
		return 0
	}
	return scaleScrollbarPosition(min(max(0, top), maxTop), maxTop, maxScroll)
}
