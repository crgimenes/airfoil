package main

import (
	"bytes"
	"image"
	_ "image/jpeg" // register JPEG decoding for image.Decode
	_ "image/png"  // register PNG decoding for image.Decode
	"os"

	"github.com/crgimenes/native/filedialog"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// refImage is a reference photo or drawing shown translucently under the
// editor canvas so the pen tool can trace over it. It is positioned in world
// (grid) coordinates like everything the editor draws, but it is a tracing
// aid only: it never reaches the solver and is never written into a saved
// scene, so opening a .afoil someone else authored never depends on it.
type refImage struct {
	img     *ebiten.Image
	path    string
	x, y    float64 // world position of the image's top-left corner
	scale   float64 // world units per image pixel
	opacity float64
	visible bool
}

// defaultRefOpacity keeps the image dim enough that a traced line over it
// stays legible.
const defaultRefOpacity = 0.5

// loadReferenceImage prompts for a PNG or JPEG and installs it as the
// editor's trace reference, fit to most of the current view and centered on
// it. Replaces any previously loaded reference.
func (g *Game) loadReferenceImage() {
	var path string
	ebiten.RunOnMainThread(func() {
		path = filedialog.Open(filedialog.Options{
			Title:      "Open reference image",
			Extensions: []string{"png", "jpg", "jpeg"},
		})
	})
	if path == "" {
		return // cancelled, or unsupported platform
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path chosen by the user via the native dialog
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	img := ebiten.NewImageFromImage(src)
	b := img.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())

	const fitFrac = 0.8 // fraction of the view the longer side should span
	viewW := float64(simW) / g.cam.zoom
	viewH := float64(simH) / g.cam.zoom
	scale := fitFrac * viewH / ih
	fitW := fitFrac * viewW / iw
	if fitW < scale {
		scale = fitW
	}
	cx, cy := g.cam.screenToWorld(simW/2, simH/2)

	g.ref = &refImage{
		img:     img,
		path:    path,
		x:       cx - iw*scale/2,
		y:       cy + ih*scale/2,
		scale:   scale,
		opacity: defaultRefOpacity,
		visible: true,
	}
	g.refPosMode = true // land in position mode so it can be aligned right away
	g.sceneErr = ""
}

// clearReference drops the loaded reference image.
func (g *Game) clearReference() {
	g.ref = nil
	g.refPosMode = false
}

// handleReferenceInput drives move (drag) and scale (wheel, anchored at the
// cursor) of the reference image while position mode is active. It reports
// whether it consumed the input, so the caller skips camera pan, object
// dragging and the pen tool for that frame.
func (g *Game) handleReferenceInput(mx, my float64, inCanvas bool) bool {
	if !g.refPosMode || g.ref == nil {
		return false
	}
	_, dy := ebiten.Wheel()
	if dy != 0 && inCanvas {
		g.scaleReferenceAt(mx, my, 1+dy*0.1)
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && inCanvas {
		g.refDragging = true
		g.refDragLastX, g.refDragLastY = mx, my
	}
	if g.refDragging && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.ref.x += (mx - g.refDragLastX) / g.cam.zoom
		g.ref.y -= (my - g.refDragLastY) / g.cam.zoom
		g.refDragLastX, g.refDragLastY = mx, my
		return true
	}
	g.refDragging = false
	return true
}

// scaleReferenceAt rescales the reference image by factor, keeping the world
// point under the cursor fixed -- the same feel as the camera's own zoom.
func (g *Game) scaleReferenceAt(sx, sy, factor float64) {
	r := g.ref
	wx, wy := g.cam.screenToWorld(sx, sy)
	b := r.img.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())
	u := (wx - r.x) / (r.scale * iw)
	v := (r.y - wy) / (r.scale * ih)
	r.scale *= factor
	if r.scale < 0.01 {
		r.scale = 0.01
	}
	r.x = wx - u*r.scale*iw
	r.y = wy + v*r.scale*ih
}

// drawReference paints the loaded trace reference under the outlines, faded
// by its opacity so a traced line stays legible over it.
func (g *Game) drawReference(vp *ebiten.Image) {
	r := g.ref
	if r == nil || !r.visible {
		return
	}
	sx, sy := g.cam.worldToScreen(r.x, r.y)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(r.scale*g.cam.zoom, r.scale*g.cam.zoom)
	op.GeoM.Translate(sx, sy)
	op.ColorScale.ScaleAlpha(float32(r.opacity))
	op.Filter = ebiten.FilterLinear
	vp.DrawImage(r.img, op)
}
