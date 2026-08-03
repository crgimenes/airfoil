package main

import (
	"math"
	"testing"
)

// TestSetControlRejectsNonFinite guards the third of the three setters every
// input path shares. math.Max/math.Min propagate a NaN rather than clamping
// it, and a NaN deflection rotates the control surface out of the rasterized
// mask, so an unchecked "CTRL nan" over UDP would delete part of the body with
// no way back. setAlpha and setSpeed have carried this guard since the UDP
// channel landed; this one arrived with CTRL.
func TestSetControlRejectsNonFinite(t *testing.T) {
	g := simGame()
	g.setScene(nekoScene(), "neko")
	want := g.controlDeg

	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		g.setControl(bad)
		if g.controlDeg != want {
			t.Fatalf("setControl(%v): controlDeg = %v, want unchanged %v", bad, g.controlDeg, want)
		}
	}

	// The guard must not be over-broad: a valid deflection still applies, and
	// an out-of-range one still clamps rather than being rejected.
	g.setControl(12)
	if g.controlDeg != 12 {
		t.Errorf("setControl(12): controlDeg = %v, want 12", g.controlDeg)
	}
	g.setControl(9999)
	if g.controlDeg != controlLimit {
		t.Errorf("setControl(9999): controlDeg = %v, want clamped to %v", g.controlDeg, controlLimit)
	}
}

// TestUDPControlChannelIsGuarded replays the wire form of the same attack, so
// the guarantee is pinned at the protocol level and not just at the setter.
func TestUDPControlChannelIsGuarded(t *testing.T) {
	g := simGame()
	g.setScene(nekoScene(), "neko")

	for _, wire := range []string{"CTRL nan", "CTRL inf", "CTRL -inf", "CTRL 1e400"} {
		g.applyControlMessage(wire)
		g.drainPending()
		if math.IsNaN(g.controlDeg) || math.IsInf(g.controlDeg, 0) {
			t.Fatalf("%q: controlDeg went non-finite (%v)", wire, g.controlDeg)
		}
	}

	solid := 0
	for _, v := range g.sceneMask(0) {
		if v {
			solid++
		}
	}
	if solid == 0 {
		t.Error("the body vanished from the mask after hostile CTRL input")
	}
}
