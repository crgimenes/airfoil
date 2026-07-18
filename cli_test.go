package main

import "testing"

// TestLoadSceneFile pins the -scene / Open shared load path (PR #8): a good
// path loads the scene and makes it the Save target; a bad path errors without
// disturbing the current scene.
func TestLoadSceneFile(t *testing.T) {
	g := simGame()
	if err := g.loadSceneFile("examples/flap.afoil"); err != nil {
		t.Fatalf("loadSceneFile: %v", err)
	}
	if g.scn == nil {
		t.Fatal("scene not set")
	}
	if g.savePath != "examples/flap.afoil" {
		t.Fatalf("savePath = %q, want the loaded file", g.savePath)
	}
	prev := g.scn
	if err := g.loadSceneFile("/nonexistent.afoil"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if g.scn != prev {
		t.Fatal("a failed load must not replace the current scene")
	}
}

// TestKioskLayout checks the -hidecontrols window shape: the clean window is the
// bare flow viewport, the normal one carries the panels.
func TestKioskLayout(t *testing.T) {
	g := &Game{}
	if w, h := g.Layout(0, 0); w != winW || h != winH {
		t.Fatalf("normal layout = %dx%d, want %dx%d", w, h, winW, winH)
	}
	g.clean = true
	if w, h := g.Layout(0, 0); w != simW || h != simH {
		t.Fatalf("kiosk layout = %dx%d, want %dx%d", w, h, simW, simH)
	}
}

// TestSetAlphaWraps pins free rotation: the angle wraps at +-180 instead of
// clamping, so arrow keys can spin the foil through a full turn.
func TestSetAlphaWraps(t *testing.T) {
	g := simGame()
	cases := []struct{ in, want float64 }{
		{45, 45},
		{90, 90},
		{180, 180},
		{185, -175},
		{-185, 175},
		{-180, 180},
		{360, 0},
		{541, -179},
	}
	for _, c := range cases {
		g.setAlpha(c.in)
		if g.alphaDeg != c.want {
			t.Errorf("setAlpha(%g): alphaDeg = %g, want %g", c.in, g.alphaDeg, c.want)
		}
	}
}
