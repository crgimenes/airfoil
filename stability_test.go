package main

import (
	"math"
	"testing"

	"kutta/lbm"
	"kutta/viz"
)

// simGame builds the minimal Game the instability path touches: solver, smoke
// and the interactive foil. No Ebiten images, so it runs headless.
func simGame() *Game {
	g := &Game{
		alphaDeg: 4,
		u0:       defaultU,
		nacaCode: profiles[0],
	}
	g.sim = lbm.New(gridW, gridH, tau, g.u0)
	g.smoke = viz.NewParticles(nParticles, gridW, gridH, 1)
	g.applyBody(true)
	return g
}

// TestStepSimResetsAfterInstability pins the backstop for issue #1: if the
// solver somehow goes non-finite, the next stepSim must rebuild a finite flow
// and clear every polluted readout instead of feeding NaN to the renderer.
func TestStepSimResetsAfterInstability(t *testing.T) {
	g := simGame()
	g.fyEMA = 123
	g.clCur = 9e99
	// Poison the solver through its public API: a NaN inlet speed writes NaN
	// populations at the boundaries, and the next collide spreads them into the
	// fields. (Corrupting Rho directly would be laundered — collide recomputes
	// the macroscopic fields from the populations every step.)
	g.sim.SetInletSpeed(math.NaN())
	g.stepSim(2)
	if !g.sim.Finite() {
		t.Fatal("solver still non-finite after the instability reset")
	}
	if g.simErr == "" {
		t.Fatal("simErr not set after the instability reset")
	}
	if g.fyEMA != 0 || g.clCur != 0 {
		t.Fatal("polluted readouts were not cleared by the reset")
	}
	// A user change acknowledges the note.
	g.setSpeed(0.05)
	if g.simErr != "" {
		t.Fatal("simErr not cleared by a user change")
	}
}

// TestSetSpeedRejectsNonFinite guards the choke point every caller of
// setSpeed goes through, including UDP control: math.Min/math.Max propagate a
// NaN instead of rejecting it, so an unchecked "SPD nan" over the network
// would permanently NaN out u0 with no UI path back.
func TestSetSpeedRejectsNonFinite(t *testing.T) {
	g := simGame()
	want := g.u0
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		g.setSpeed(bad)
		if g.u0 != want {
			t.Fatalf("setSpeed(%v): u0 = %v, want unchanged %v", bad, g.u0, want)
		}
		if !g.sim.Finite() {
			t.Fatalf("setSpeed(%v): solver went non-finite", bad)
		}
	}
}

// TestSetAlphaRejectsNonFinite mirrors TestSetSpeedRejectsNonFinite for the
// angle-of-attack path: math.Mod passes a NaN straight through (it compares
// false against both wrap bounds), so an unchecked "AOA inf" would
// permanently NaN out alphaDeg.
func TestSetAlphaRejectsNonFinite(t *testing.T) {
	g := simGame()
	want := g.alphaDeg
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		g.setAlpha(bad)
		if g.alphaDeg != want {
			t.Fatalf("setAlpha(%v): alphaDeg = %v, want unchanged %v", bad, g.alphaDeg, want)
		}
	}
}

// TestInstabilityResetKeepsSceneBody guards the scene-mode gap: the reset must
// rebuild the flow around the LOADED SCENE's mask, not the interactive foil's.
func TestInstabilityResetKeepsSceneBody(t *testing.T) {
	g := simGame()
	g.scn = nekoScene()
	g.sim.SetSolid(g.sceneMask(0))

	g.sim.SetInletSpeed(math.NaN())
	g.stepSim(2)

	if !g.sim.Finite() {
		t.Fatal("solver still non-finite after the scene-mode reset")
	}
	want := g.sceneMask(0)
	for y := range gridH {
		for x := range gridW {
			if g.sim.Solid(x, y) != want[y*gridW+x] {
				t.Fatalf("solid mask diverged from the scene at (%d,%d): reset used the wrong body", x, y)
			}
		}
	}
}
