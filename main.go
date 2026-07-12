// Command kutta is a 2D wind-tunnel toy: it streams a Lattice-Boltzmann flow
// past a NACA airfoil and visualizes speed, vorticity, smoke streaklines and the
// lift/drag vectors, with an adjustable angle of attack.
package main

import (
	"flag"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

// windowTitle names the app window; the Windows menu backend also uses it to
// locate the window handle, so it must stay unique to this process.
const windowTitle = "kutta — 2D wind tunnel"

func main() {
	kiosk := flag.Bool("kiosk", false, "start fullscreen in kiosk mode (no menu, no panels)")
	kioskControls := flag.Bool("kiosk-controls", false, "in kiosk mode, keep the AoA/speed/control sliders visible and usable")
	flag.Parse()

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	setWindowIcon()

	g := NewGame()
	if *kiosk {
		g.enterKiosk(*kioskControls)
	}

	err := ebiten.RunGame(g)
	if err != nil {
		log.Fatal(err)
	}
}
