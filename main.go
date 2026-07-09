// Command kutta is a 2D wind-tunnel toy: it streams a Lattice-Boltzmann flow
// past a NACA airfoil and visualizes speed, vorticity, smoke streaklines and the
// lift/drag vectors, with an adjustable angle of attack.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

// windowTitle names the app window; the Windows menu backend also uses it to
// locate the window handle, so it must stay unique to this process.
const windowTitle = "kutta — 2D wind tunnel"

func main() {
	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	setWindowIcon()
	err := ebiten.RunGame(NewGame())
	if err != nil {
		log.Fatal(err)
	}
}
