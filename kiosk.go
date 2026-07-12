package main

import "github.com/hajimehoshi/ebiten/v2"

// enterKiosk switches to fullscreen kiosk mode: just the flow (plus the
// AoA/speed/control-surface sliders when controls is true), no menu, no
// panels, no shape editor. Whatever is currently loaded -- the interactive
// foil or a scene -- keeps running; kiosk mode only changes what is drawn and
// which inputs are live, so leaving it picks up right where it left off.
func (g *Game) enterKiosk(controls bool) {
	g.editing = false
	g.kiosk = true
	g.kioskControls = controls
	g.gui.ClearFocus()
	g.side.ClearFocus()
	ebiten.SetFullscreen(true)
}

// exitKiosk returns to the normal windowed editor/simulator.
func (g *Game) exitKiosk() {
	g.kiosk = false
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
