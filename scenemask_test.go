package main

import (
	"testing"

	"kutta/foil"
	"kutta/scene"
)

// referenceSceneMask is the original allocating mask build, kept as the
// reference the scratch-buffer path must match cell for cell.
func referenceSceneMask(g *Game, t float64) []bool {
	mask := make([]bool, gridW*gridH)
	for _, o := range g.scn.Objects {
		if o.Broken() {
			continue
		}
		poly := o.PolygonAt(t)
		if o.Control {
			poly = scene.Apply(o.Outline(), o.Pivot, scene.Pose{Rot: g.controlDeg, Scale: 1})
		}
		foil.RasterizeInto(mask, g.sceneGlobal(poly), gridW, gridH)
	}
	return mask
}

// TestSceneMaskMatchesReference pins the zero-alloc mask path to the original
// one across timeline positions, angles, and the live control deflection.
func TestSceneMaskMatchesReference(t *testing.T) {
	g := simGame()
	g.setScene(nekoScene(), "neko")
	g.alphaDeg = 7.5
	g.controlDeg = 12

	for _, tt := range []float64{0, 0.4, 1.3, 2.7} {
		got := g.sceneMask(tt)
		want := referenceSceneMask(g, tt)
		for c := range want {
			if got[c] != want[c] {
				t.Fatalf("t=%v: mask differs at cell %d", tt, c)
			}
		}
	}

	// The buffer is shared: a second call must still be self-consistent.
	a := g.sceneMask(0.4)
	count := 0
	for _, v := range a {
		if v {
			count++
		}
	}
	if count == 0 {
		t.Fatal("mask came back empty")
	}
}
