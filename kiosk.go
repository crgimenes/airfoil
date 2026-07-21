package main

import "github.com/hajimehoshi/ebiten/v2"

// enterKiosk switches to fullscreen kiosk mode: just the flow (plus the
// AoA/speed/control-surface sliders when controls is true), no menu, no
// panels, no shape editor. Whatever is currently loaded -- the interactive
// foil or a scene -- keeps running; kiosk mode only changes what is drawn and
// which inputs are live, so leaving it picks up right where it left off.
//
// clean is the same flag -hidecontrols sets on its own (hide the panels,
// nothing else); kiosk is the lockout on top of that (trimmed menu bar,
// Escape/Open/Save/the shape editor all blocked) that -hidecontrols alone
// must not gain -- it's for clean recording frames, not locking the operator
// out of their own file dialogs.
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
	g.kiosk = true
	g.kioskControls = controls
	g.gui.ClearFocus()
	g.side.ClearFocus()
	g.startFullscreen = true
}

// exitKiosk returns to the normal windowed editor/simulator.
func (g *Game) exitKiosk() {
	g.clean = false
	g.kiosk = false
	g.startFullscreen = false
	ebiten.SetFullscreen(false)
}

// toggleKiosk flips kiosk mode, remembering the controls variant last used --
// the Ctrl+Shift+K escape hatch, in both directions.
func (g *Game) toggleKiosk() {
	if g.kiosk {
		g.exitKiosk()
		return
	}
	g.enterKiosk(g.kioskControls)
}
