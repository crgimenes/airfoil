package main

import "testing"

// TestWarpMultipliesSimTime pins what -warp means: warp N runs N times the
// solver steps per second, at any tick rate, while a bare Game stays at real
// time.
func TestWarpMultipliesSimTime(t *testing.T) {
	base := simGame()
	baseSteps := base.substepsPerTick() * base.tickTPS()

	for _, warp := range []int{1, 2, 3, 4} {
		for _, tps := range []int{30, 60} {
			g := simGame()
			g.tps = tps
			g.warp = warp
			steps := g.substepsPerTick() * g.tickTPS()
			if steps != baseSteps*warp {
				t.Errorf("warp=%d tps=%d: %d solver steps/s, want %d", warp, tps, steps, baseSteps*warp)
			}
		}
	}
}

// TestWarpLeavesTheClockAlone pins the two deliberate non-scalings: the
// animation timeline and the readout smoothing stay on wall-clock time, since
// a flap in a faster wind still moves at its own pace and the panel numbers
// should not get jumpier.
func TestWarpLeavesTheClockAlone(t *testing.T) {
	g := simGame()
	g.warp = 3
	if g.tickScale() != 1 {
		t.Errorf("tickScale = %v under warp, want 1 (timeline must stay on the clock)", g.tickScale())
	}
	if g.emaAlphaPerTick() != 0.04 {
		t.Errorf("ema alpha = %v under warp, want the baseline 0.04", g.emaAlphaPerTick())
	}
}

// TestWarpValidation pins the accepted range.
func TestWarpValidation(t *testing.T) {
	for _, w := range []int{1, 2, 3, 4} {
		if !validWarp(w) {
			t.Errorf("validWarp(%d) = false, want accepted", w)
		}
	}
	for _, w := range []int{0, -1, 5, 100} {
		if validWarp(w) {
			t.Errorf("validWarp(%d) = true, want refused", w)
		}
	}
	g := simGame()
	if g.warpFactor() != 1 {
		t.Errorf("bare Game warpFactor = %d, want 1", g.warpFactor())
	}
}
