package main

import (
	"math"
	"testing"
)

// TestTickRateKeepsPhysicsRate pins the invariant behind -tps: whatever the
// tick rate, one wall-clock second must advance the solver, the timeline and
// the smoke by exactly the same amount as the 60-tick baseline.
func TestTickRateKeepsPhysicsRate(t *testing.T) {
	for _, tps := range []int{10, 12, 15, 20, 30, 36, 45, 60} {
		if !validTPS(tps) {
			t.Errorf("validTPS(%d) = false, want it accepted", tps)
			continue
		}
		g := simGame()
		g.tps = tps

		stepsPerSec := g.substepsPerTick() * tps
		if stepsPerSec != substeps*60 {
			t.Errorf("tps=%d: %d solver steps/s, want %d", tps, stepsPerSec, substeps*60)
		}
		animPerSec := animDt * g.tickScale() * float64(tps)
		if math.Abs(animPerSec-animDt*60) > 1e-12 {
			t.Errorf("tps=%d: timeline advances %.6f/s, want %.6f", tps, animPerSec, animDt*60)
		}
		tracerPerSec := tracerSpeed * g.tickScale() * float64(tps)
		if math.Abs(tracerPerSec-tracerSpeed*60) > 1e-9 {
			t.Errorf("tps=%d: smoke advects %.3f/s, want %.3f", tps, tracerPerSec, tracerSpeed*60)
		}
	}
}

// TestTickRateRejectsUnevenRates pins the validation: a rate that does not
// divide the physics rate would drift the flow speed, so it is refused.
func TestTickRateRejectsUnevenRates(t *testing.T) {
	for _, tps := range []int{0, 7, 25, 50, 61, 120} {
		if validTPS(tps) {
			t.Errorf("validTPS(%d) = true, want it refused", tps)
		}
	}
}

// TestTickRateDefaultsWithoutFlag pins the zero value: a bare Game behaves
// exactly as the 60-tick baseline, so nothing else in the app or the tests has
// to know the feature exists.
func TestTickRateDefaultsWithoutFlag(t *testing.T) {
	g := simGame()
	if g.tickTPS() != 60 || g.substepsPerTick() != substeps || g.tickScale() != 1 {
		t.Errorf("bare Game: tps=%d substeps=%d scale=%v, want 60/%d/1",
			g.tickTPS(), g.substepsPerTick(), g.tickScale(), substeps)
	}
	if g.emaAlphaPerTick() != 0.04 {
		t.Errorf("bare Game: ema alpha = %v, want the baseline 0.04", g.emaAlphaPerTick())
	}
}

// TestTickRateEMAEquivalence pins the smoothing compensation: after one second
// of a constant input, the smoothed readout must land at the same value at 30
// ticks as at 60, within float noise.
func TestTickRateEMAEquivalence(t *testing.T) {
	settle := func(tps int) float64 {
		g := simGame()
		g.tps = tps
		a := g.emaAlphaPerTick()
		v := 0.0
		for range tps {
			v += a * (1 - v)
		}
		return v
	}
	v60 := settle(60)
	v30 := settle(30)
	if math.Abs(v60-v30) > 1e-9 {
		t.Errorf("after 1s of constant input: ema(60)=%v ema(30)=%v, want equal", v60, v30)
	}
}
