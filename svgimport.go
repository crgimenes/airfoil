package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crgimenes/native/filedialog"
	"github.com/hajimehoshi/ebiten/v2"

	"kutta/foil"
	"kutta/scene"
)

// svgImportScene builds a static scene from normalized SVG outlines (x in
// [0,1], y up, vertical center 0 — what foil.ParseSVG returns), one object per
// outline. The group is placed like the interactive foil: chordFrac of the
// grid wide, left edge at leadXFrac, vertically centered; one shared mapping
// keeps the outlines' drawn relative positions. Each object pivots about its
// own centroid so it rotates naturally in the editor.
func svgImportScene(outlines [][]foil.Point, name string) *scene.Scene {
	chord := chordFrac * gridW
	leadX := leadXFrac * gridW
	s := &scene.Scene{}
	for i, out := range outlines {
		placed := foil.Place(out, chord, leadX, float64(gridH)/2, 0, 0)
		objName := name
		if len(outlines) > 1 {
			objName = fmt.Sprintf("%s-%d", name, i+1)
		}
		s.Objects = append(s.Objects, &scene.Object{
			Name:  objName,
			Shape: placed,
			Pivot: centroid(placed),
		})
	}
	return s
}

// importSVGDialog asks for an SVG file and loads its outlines as a new scene.
// The scene has no save path yet: Save falls through to Save As, writing a
// normal .afoil file with the imported geometry baked in as shape points.
func (g *Game) importSVGDialog() {
	// Native panel on the main thread, same dance as openSceneDialog.
	var path string
	ebiten.RunOnMainThread(func() {
		path = filedialog.Open(filedialog.Options{
			Title:      "Import SVG drawing",
			Extensions: []string{"svg"},
		})
	})
	if path == "" {
		g.noDialogHint()
		return // cancelled, or unsupported platform
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path chosen by the user via the native dialog
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	outlines, err := foil.ParseSVG(data)
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	g.setScene(svgImportScene(outlines, name), path)
}
