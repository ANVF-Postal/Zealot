package main

import (
	"sync"

	tea "charm.land/bubbletea/v2"
)

// mouseMotionState stores the newest terminal pointer coordinate and whether
// a frame refresh is already queued. The filter and Bubble Tea update loop run
// on different goroutines, so all shared state is protected by this mutex.
type mouseMotionState struct {
	mu      sync.Mutex
	x       int
	y       int
	known   bool
	pending bool
}

func (s *mouseMotionState) record(x, y int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.x = x
	s.y = y
	s.known = true
	s.mu.Unlock()
}

// capture stores every incoming pointer position, but only admits the first
// event in a frame. Intermediate motion can be discarded because the latest
// coordinate fully describes the pointer state needed by hover and dragging.
func (s *mouseMotionState) capture(x, y int) bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.x = x
	s.y = y
	s.known = true
	if s.pending {
		return false
	}
	s.pending = true
	return true
}

// position returns the latest pointer coordinate without releasing the
// pending frame slot. Mouse release uses this to apply the final drag position
// before it ends the drag, while leaving the scheduled frame intact.
func (s *mouseMotionState) position() (x, y int, known bool) {
	if s == nil {
		return 0, 0, false
	}
	s.mu.Lock()
	x, y, known = s.x, s.y, s.known
	s.mu.Unlock()
	return
}

// takeAndRelease returns the newest position and opens the slot for the next
// frame. It is called only when the one scheduled frame message is handled.
func (s *mouseMotionState) takeAndRelease() (x, y int, known bool) {
	if s == nil {
		return 0, 0, false
	}
	s.mu.Lock()
	x, y, known = s.x, s.y, s.known
	s.pending = false
	s.mu.Unlock()
	return
}

type mouseMotionFilter struct {
	state *mouseMotionState
}

// filter bounds pointer-driven work to one update per frame, including active
// scrollbar drags and text selection. Clicks, releases, key events and all
// non-motion messages are always passed through immediately.
func (f mouseMotionFilter) filter(_ tea.Model, msg tea.Msg) tea.Msg {
	motion, ok := msg.(tea.MouseMotionMsg)
	if !ok || f.state == nil {
		return msg
	}
	if !f.state.capture(motion.X, motion.Y) {
		return nil
	}
	return msg
}
