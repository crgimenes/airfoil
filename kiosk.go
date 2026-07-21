package main

import "github.com/hajimehoshi/ebiten/v2"

// enterKiosk switches to fullscreen kiosk mode: just the flow (plus the
// AoA/speed/control-surface sliders when controls is true), no menu, no
// panels, no shape editor. Whatever is currently loaded -- the interactive
// foil or a scene -- keeps running; kiosk mode only changes what is drawn and
// which inputs are live, so leaving it picks up right where it left off.
//
// Fullscreen is requested via startFullscreen/fsCountdown rather than calling
// ebiten.SetFullscreen(true) directly: entering kiosk mode at startup (before
// the window exists) hits the same macOS black-frame bug -fullscreen works
// around, and reusing that path fixes it for kiosk mode too. Toggled at
// runtime, fsCountdown is already past its threshold, so it takes effect on
// the very next frame -- no perceptible delay.
func (g *Game) enterKiosk(controls bool) {
	g.editing = false
	g.clean = true
	g.kioskControls = controls
	g.gui.ClearFocus()
	g.side.ClearFocus()
	g.startFullscreen = true
}

// exitKiosk returns to the normal windowed editor/simulator.
func (g *Game) exitKiosk() {
	g.clean = false
	g.startFullscreen = false
	ebiten.SetFullscreen(false)
}

// toggleKiosk flips kiosk mode, remembering the controls variant last used --
// the Ctrl+Shift+K escape hatch, in both directions.
func (g *Game) toggleKiosk() {
	if g.clean {
		g.exitKiosk()
		return
	}
	g.enterKiosk(g.kioskControls)
}
