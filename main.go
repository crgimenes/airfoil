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
	udpAddr := flag.String("udp", "", "listen address (e.g. :9000) for UDP slider control from external hardware; disabled if empty")
	flag.Parse()

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	setWindowIcon()

	g := NewGame()
	if *udpAddr != "" {
		err := g.startUDPControl(*udpAddr)
		if err != nil {
			log.Printf("kutta: -udp %q: %v", *udpAddr, err)
		}
	}

	err := ebiten.RunGame(g)
	if err != nil {
		log.Fatal(err)
	}
}
