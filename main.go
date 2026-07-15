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
	scenePath := flag.String("scene", "", "path to an .afoil scene file to load at startup instead of the interactive default foil")
	flag.Parse()

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	setWindowIcon()

	g := NewGame()
	if *scenePath != "" {
		err := g.loadSceneFile(*scenePath)
		if err != nil {
			log.Printf("kutta: -scene %q: %v", *scenePath, err)
		}
	}

	err := ebiten.RunGame(g)
	if err != nil {
		log.Fatal(err)
	}
}
