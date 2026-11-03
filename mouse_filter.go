package main

import (
	"sync"

	tea "charm.land/bubbletea/v2"
)

// mouseMotionState shares the latest terminal pointer position with the model.
type mouseMotionState struct {
	mu    sync.Mutex
	x     int
	y     int
	known bool
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

func (s *mouseMotionState) position() (x, y int, known bool) {
	if s == nil {
		return 0, 0, false
	}
	s.mu.Lock()
	x, y, known = s.x, s.y, s.known
	s.mu.Unlock()
	return
}

type mouseMotionFilter struct {
	state *mouseMotionState
}

// filter records pointer coordinates but always lets motion through. Passive
// hover is now cheap enough to update immediately, and active scrollbar drags
// must never be throttled or discarded.
func (f mouseMotionFilter) filter(_ tea.Model, msg tea.Msg) tea.Msg {
	if motion, ok := msg.(tea.MouseMotionMsg); ok && f.state != nil {
		f.state.record(motion.X, motion.Y)
	}
	return msg
}
