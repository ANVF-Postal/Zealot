package main

// doomFireTip is a sparse rising wisp above the compact Doom heat grid.
// Its position is derived from the fixed simulation clock: no second heat
// buffer, particle state, or per-frame random generator is required.
type doomFireTip struct {
	x, y int
	level uint8
}

func doomFireTips(state *doomFireState, viewportHeight int, salt uint64) []doomFireTip {
	if state == nil || state.width < 1 || viewportHeight <= state.height {
		return nil
	}
	top := viewportHeight - state.height
	count := max(2, min(48, state.width/7))
	tips := make([]doomFireTip, 0, count)
	tick := state.step / 4 // Same 7.5 Hz clock as the heat propagation.
	for i := 0; i < count; i++ {
		seed := chatFXHash(salt + uint64(i+1)*0x9e3779b97f4a7c15)
		lifetime := uint64(30 + chatFXHash(seed+1)%44)
		age := (tick + chatFXHash(seed+2)%lifetime) % lifetime
		// Different lifetimes and heights prevent a second flat edge.
		rise := max(1, int(float64(top+1)*(0.35+0.65*chatFXUnit(seed+3))))
		y := top + 1 - int(age*uint64(rise+2)/lifetime)
		if y < 0 || y >= top {
			continue
		}
		x := int(chatFXHash(seed+4) % uint64(state.width))
		// Wisps originate over genuinely hot columns near the flame body.
		sourceY := max(0, state.height-1-min(4, state.height-1))
		if state.heat[sourceY*state.width+x] < 8 {
			continue
		}
		// Slight horizontal drift, shared with the slowly swaying flame.
		drift := int(age/14)
		if fireWind(tick, salt) < 0 {
			drift = -drift
		}
		x += drift
		if x < 0 || x >= state.width {
			continue
		}
		level := uint8(5 + chatFXHash(seed+5+tick/12)%8)
		tips = append(tips, doomFireTip{x: x, y: y, level: level})
	}
	return tips
}
