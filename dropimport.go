package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"kutta/foil"
	"kutta/scene"
	"kutta/sceneio"
)

// noDialogHint explains a dialog that returned nothing on platforms where the
// native open panel is a stub (everywhere but macOS), pointing at the
// drag-and-drop path that works instead. On macOS an empty result is the user
// cancelling, which deserves silence.
func (g *Game) noDialogHint() {
	if onWeb {
		g.sceneErr = "drop an .afoil or .svg file onto the tunnel to load it"
		return
	}
	if runtime.GOOS != "darwin" {
		g.sceneErr = "no file dialog on this OS yet — drop the file onto the window instead"
	}
}

// sceneFromFile builds a scene from a file dropped onto the window, dispatching
// on the extension: .svg imports a drawing, .afoil loads a scene file.
func sceneFromFile(name string, data []byte) (*scene.Scene, error) {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	switch strings.ToLower(filepath.Ext(name)) {
	case ".svg":
		outlines, err := foil.ParseSVG(data)
		if err != nil {
			return nil, err
		}
		return svgImportScene(outlines, base), nil
	case sceneio.Ext:
		return sceneio.Load(string(data))
	}
	return nil, fmt.Errorf("%q: only %s scenes and .svg drawings can be dropped here", filepath.Ext(name), sceneio.Ext)
}

// handleDroppedFiles opens a scene or SVG drawing dragged onto the window, the
// file-open path that needs no native dialog (Windows and Linux have none yet).
// The first loadable file wins; an error lands in the toolbar like a failed
// Open… would.
func (g *Game) handleDroppedFiles() {
	files := ebiten.DroppedFiles()
	if files == nil {
		return
	}
	err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(files, path)
		if err != nil {
			return err
		}
		sc, err := sceneFromFile(path, data)
		if err != nil {
			return err
		}
		// The virtual FS exposes base names, not real paths, so the drop sets
		// no save target: Save falls through to Save As like any new scene.
		g.setScene(sc, path)
		return fs.SkipAll
	})
	if err != nil {
		g.sceneErr = err.Error()
	}
}
