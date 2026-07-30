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
	glow := flag.Bool("glow", true, "additive bloom on the smoke")
	streamlines := flag.Bool("streamlines", false, "overlay integrated streamlines")
	mode := flag.String("mode", "", "field display at startup: speed, vorticity, or pressure (default speed)")
	udpAddr := flag.String("udp", "", "listen address (e.g. :9000) for UDP slider control from external hardware; disabled if empty")
	debug := flag.Bool("debug", false, "log per-second performance lines and events to the terminal")
	tps := flag.Int("tps", 60, "simulation ticks per second; lower values (e.g. 30) free a slow machine to draw more frames, with the flow speed unchanged")
	scenePath := flag.String("scene", "", "path to an .afoil scene file to load at startup instead of the interactive default foil")
	fullscreen := flag.Bool("fullscreen", false, "start in full screen")
	hideControls := flag.Bool("hidecontrols", false, "hide every panel and control, showing only the flow image (kiosk mode)")
	flag.Parse()

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle(windowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	setWindowIcon()

	g := NewGame()
	g.perf.enabled = *debug
	if *tps != 60 {
		if validTPS(*tps) {
			g.tps = *tps
			ebiten.SetTPS(*tps)
		} else {
			log.Printf("kutta: -tps %d: not a clean divisor of the physics rate; keeping 60", *tps)
		}
	}
	g.glow = *glow
	g.streamlines = *streamlines
	if *mode != "" {
		fm, ok := parseFieldMode(*mode)
		if !ok {
			log.Printf("kutta: -mode %q: not one of speed, vorticity, pressure; leaving the default", *mode)
		} else {
			g.mode = fm
		}
	}
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
	if *kiosk {
		g.enterKiosk(*kioskControls)
	}
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
