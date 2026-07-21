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
	fullscreen := flag.Bool("fullscreen", false, "start in full screen")
	hideControls := flag.Bool("hidecontrols", false, "hide every panel and control, showing only the flow image (kiosk mode)")
	flag.Parse()

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	setWindowIcon()
	g := NewGame()
	g.clean = *hideControls
	// Fullscreen is applied on the first Update, not here: entering fullscreen
	// before the window exists leaves the first frame black on macOS until a
	// resize. Deferring reproduces the toggle-after-launch path, which works.
	g.startFullscreen = *fullscreen
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
