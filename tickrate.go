package main

import "math"

// Tick-rate rebudget for machines that cannot update and draw 60 times a
// second. John's Pi 5 log made the waste visible: with the solver fast enough
// to hold 60 ticks, the main thread spent 544 ms of every second computing 60
// simulation updates of which only 17 reached the screen.
//
// Lowering the tick rate does not slow the flow: the physics rate is
// substeps x tps solver steps per second, so -tps 30 doubles the substeps per
// tick and the flow, the timeline and the smoke all advance exactly as at 60.
// What changes is the budget: half the ticks frees the thread to draw twice
// the frames. The one real cost is input latency, one tick, 33 ms at 30.

// maxWarp bounds the time multiplier. Four is already beyond what any current
// machine sustains at 60 ticks; the bound exists so a typo cannot ask for a
// thousandfold simulation.
const maxWarp = 4

// validWarp reports whether w is a usable time multiplier.
func validWarp(w int) bool {
	return w >= 1 && w <= maxWarp
}

// warpFactor is the simulation-time multiplier: how many times faster than
// real time the flow evolves. The wind does not change, time does, so the
// solver stays at the same stable operating point while the smoke, the wake
// and the shedding all move proportionally faster on screen. The cost is
// linear CPU: each tick runs warp times the solver steps.
func (g *Game) warpFactor() int {
	if g.warp <= 1 {
		return 1
	}
	return g.warp
}

// validTPS reports whether the physics rate divides cleanly into tps ticks, so
// every derived quantity below stays exact.
func validTPS(tps int) bool {
	return tps >= 10 && tps <= 60 && (substeps*60)%tps == 0
}

// tickTPS is the configured tick rate, defaulting to the standard 60 so tests
// and benchmarks that build a bare Game are unaffected.
func (g *Game) tickTPS() int {
	if g.tps <= 0 {
		return 60
	}
	return g.tps
}

// tickScale is how much longer one tick lasts compared to the 60-a-second
// baseline; every per-tick advance multiplies by it.
func (g *Game) tickScale() float64 {
	return 60 / float64(g.tickTPS())
}

// substepsPerTick keeps the solver at the same steps per second regardless of
// the tick rate, times the warp factor when simulation time is accelerated.
func (g *Game) substepsPerTick() int {
	return substeps * 60 / g.tickTPS() * g.warpFactor()
}

// emaAlphaPerTick compensates the force-readout smoothing so its time constant
// stays the same wall-clock length at any tick rate.
func (g *Game) emaAlphaPerTick() float64 {
	const perTick60 = 0.04
	if g.tickTPS() == 60 {
		return perTick60
	}
	return 1 - math.Pow(1-perTick60, g.tickScale())
}
