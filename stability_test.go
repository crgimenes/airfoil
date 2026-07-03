package main

import (
	"math"
	"testing"
)

func TestResetFlowAfterInstability(t *testing.T) {
	g := NewGame()
	g.sim.Rho[0] = math.NaN()
	if solverFinite(g.sim) {
		t.Fatal("solverFinite() = true after injecting NaN")
	}

	g.resetFlowAfterInstability()
	if !solverFinite(g.sim) {
		t.Fatal("solver is still non-finite after reset")
	}
	if g.simErr == "" {
		t.Fatal("simErr is empty after instability reset")
	}
	if g.clCur != 0 || g.cdCur != 0 || g.fxEMA != 0 || g.fyEMA != 0 || g.mzEMA != 0 || g.sepEMA != 0 {
		t.Fatal("displayed simulation values were not cleared")
	}
}
