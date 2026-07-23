// Command anim is the Phase-1 end-to-end check for the scene/animation spine,
// run headlessly: it loads a Filo scene with an animated flap, drives the solver
// by swapping the union solid mask over time (the same UpdateSolid path the live
// app uses), and writes PNGs plus a lift readout proving the flow reacts. It is
// a development tool, not part of the app.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"kutta/lbm"
	"kutta/scene"
	"kutta/sceneio"
	"kutta/viz"
)

const (
	nx, ny = 360, 200
	u0     = 0.10
	chord  = 130.0
)

func main() {
	outDir := flag.String("out", ".", "directory to write the output PNGs into")
	scenePath := flag.String("scene", "examples/flap.afoil", "Filo scene file to load")
	flag.Parse()

	src, err := os.ReadFile(*scenePath) // #nosec G304 -- path is an explicit CLI arg
	if err != nil {
		panic(err)
	}
	sc, err := sceneio.Load(string(src))
	if err != nil {
		panic(err)
	}
	fmt.Printf("loaded scene: %d objects, loop %.0fs\n", len(sc.Objects), sc.Loop)

	// Clean snapshots of the two extremes (fresh settle each), for the eye.
	pRet := filepath.Join(*outDir, "anim_flap_retracted.png")
	pExt := filepath.Join(*outDir, "anim_flap_extended.png")
	clRetracted := settleAndShoot(sc, 0, pRet)
	clExtended := settleAndShoot(sc, 2, pExt)
	fmt.Printf("retracted (t=0): Cl ~ %+.3f -> %s\n", clRetracted, abs(pRet))
	fmt.Printf("extended  (t=2): Cl ~ %+.3f -> %s\n", clExtended, abs(pExt))
	if clExtended <= clRetracted {
		fmt.Println("WARNING: extending the flap did not raise lift")
	}

	// Continuous animation: warm up, then move the flap via UpdateSolid every
	// few steps over one loop, printing the lift as it goes.
	fmt.Println("continuous animation (UpdateSolid per frame):")
	animate(sc)
}

// settleAndShoot rebuilds the body at scene time t, lets the flow settle from
// scratch, writes a speed-field PNG, and returns the lift coefficient.
func settleAndShoot(sc *scene.Scene, t float64, path string) float64 {
	s := lbm.New(nx, ny, 0.6, u0)
	s.SetSolid(sc.Mask(t, nx, ny))
	for range 6000 {
		s.Step()
	}
	writeSpeedPNG(path, s)
	return s.Fy / (0.5 * u0 * u0 * chord)
}

// animate plays the loop continuously, swapping the mask in place so the wake
// carries over — the runtime the live app will use.
func animate(sc *scene.Scene) {
	s := lbm.New(nx, ny, 0.6, u0)
	s.SetSolid(sc.Mask(0, nx, ny))
	for range 3000 { // establish a flow first
		s.Step()
	}
	const stepsPerLoop = 2400
	denom := 0.5 * u0 * u0 * chord
	for step := 0; step <= stepsPerLoop; step++ {
		t := sc.LoopTime(float64(step) / stepsPerLoop * sc.Loop)
		if step%30 == 0 {
			s.UpdateSolid(sc.Mask(t, nx, ny))
		}
		s.Step()
		if step%300 == 0 {
			fmt.Printf("  t=%.2fs  Cl ~ %+.3f\n", t, s.Fy/denom)
		}
	}
}

func writeSpeedPNG(path string, s *lbm.Solver) {
	img := image.NewRGBA(image.Rect(0, 0, nx, ny))
	for y := range ny {
		row := ny - 1 - y
		for x := range nx {
			sp := math.Hypot(s.Ux[y*nx+x], s.Uy[y*nx+x])
			c := viz.Speed(sp / (u0 * 2))
			if s.Solid(x, y) {
				c = color.RGBA{0x1a, 0x1d, 0x24, 0xff}
			}
			img.Set(x, row, c)
		}
	}
	f, err := os.Create(path) // #nosec G304 -- output path is an explicit CLI arg
	if err != nil {
		panic(err)
	}
	err = png.Encode(f, img)
	if err != nil {
		panic(err)
	}
	err = f.Close()
	if err != nil {
		panic(err)
	}
}

// abs returns the absolute form of p for printing, falling back to p.
func abs(p string) string {
	a, err := filepath.Abs(p)
	if err == nil {
		return a
	}
	return p
}
