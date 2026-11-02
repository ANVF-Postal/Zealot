package main

import (
	"sync"

	tea "charm.land/bubbletea/v2"
)

// mouseMotionState shares the latest terminal pointer position between
// Bubble Tea's input filter and the delayed hover-refresh message.
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

// filter runs before Update and View. While a passive hover refresh is
// pending, motion events only update the shared pointer position; they do not
// trigger another model update or render. Clicks and active drags always pass.
func (f mouseMotionFilter) filter(current tea.Model, msg tea.Msg) tea.Msg {
	motion, ok := msg.(tea.MouseMotionMsg)
	if !ok {
		return msg
	}
	if f.state != nil {
		f.state.record(motion.X, motion.Y)
	}

	m, ok := current.(model)
	if !ok {
		return msg
	}
	if motion.Button != 0 || m.inputSelecting || m.scrollDragging || m.usersScrollDragging || m.splashScrollDragging {
		return msg
	}
	if m.messageHoverPending {
		return nil
	}
	return msg
}
