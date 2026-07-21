package main

import (
	"strings"
	"testing"
)

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

// TestHideControlsAloneKeepsFullMenu pins the split Cesar asked for on PR #18:
// -hidecontrols (clean alone) must keep meaning exactly what it means today --
// hide the panels, nothing else -- so the full menu (and by extension every
// hotkey gated on it) stays intact. Only kiosk (only ever set together with
// clean, via enterKiosk) trims the menu down to Quit/Exit Kiosk Mode.
func TestHideControlsAloneKeepsFullMenu(t *testing.T) {
	g := editGame()
	g.clean = true
	want := []string{"File", "Edit", "View", "Foil", "Animate"}
	for _, w := range want {
		if !strings.Contains(menuTitles(g), w) {
			t.Errorf("-hidecontrols alone (clean, no kiosk) menu missing %q (got %q)", w, menuTitles(g))
		}
	}
	g.kiosk = true
	items := g.menuItems()
	if len(items) != 1 || items[0].Title != "kutta" {
		t.Fatalf("kiosk menu should be trimmed to a single 'kutta' entry, got %q", menuTitles(g))
	}
	var subTitles []string
	for _, it := range items[0].Submenu {
		subTitles = append(subTitles, it.Title)
	}
	if !strings.Contains(strings.Join(subTitles, " "), "Exit Kiosk Mode") {
		t.Errorf("kiosk submenu missing Exit Kiosk Mode, got %v", subTitles)
	}
}

// TestKioskToggleSetsBothFlags pins enterKiosk/exitKiosk/toggleKiosk always
// moving clean and kiosk together (unlike -hidecontrols, which only ever
// touches clean): kiosk mode should hide panels AND apply the lockout in one
// step, and leaving it should restore both, not just one.
func TestKioskToggleSetsBothFlags(t *testing.T) {
	g := simGame()

	g.enterKiosk(false)
	if !g.clean || !g.kiosk || g.kioskControls {
		t.Fatalf("enterKiosk(false): clean=%v kiosk=%v kioskControls=%v, want true/true/false", g.clean, g.kiosk, g.kioskControls)
	}

	g.exitKiosk()
	if g.clean || g.kiosk {
		t.Fatalf("exitKiosk: clean=%v kiosk=%v, want both false", g.clean, g.kiosk)
	}

	g.toggleKiosk() // re-enter, remembering kioskControls=false
	if !g.clean || !g.kiosk || g.kioskControls {
		t.Fatalf("toggleKiosk (enter): clean=%v kiosk=%v kioskControls=%v, want true/true/false", g.clean, g.kiosk, g.kioskControls)
	}
	g.toggleKiosk() // exit
	if g.clean || g.kiosk {
		t.Fatalf("toggleKiosk (exit): clean=%v kiosk=%v, want both false", g.clean, g.kiosk)
	}

	g.enterKiosk(true)
	g.toggleKiosk() // exit
	g.toggleKiosk() // re-enter, should remember kioskControls=true
	if !g.kioskControls {
		t.Fatal("toggleKiosk re-entry did not remember the controls variant")
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
