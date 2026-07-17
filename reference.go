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

// Reference backdrop: a photo or scanned drawing shown translucently under
// the editor canvas so the pen tool can trace over it. It is positioned in
// world (grid) coordinates like everything else the editor draws, but it is
// ONLY a drawing aid: kept in memory, never part of the scene, so it never
// reaches the solver and opening a .afoil someone else authored never
// depends on it. Same role as linefire's mapeditor/backdrop.go, which this
// mirrors.
type backdropImage struct {
	img     *ebiten.Image
	path    string
	x, y    float64 // world position of the image's top-left corner
	scale   float64 // world units per image pixel
	opacity float64
	visible bool
}

// backdropDefaultAlpha is the starting opacity: faint enough that a traced
// line reads clearly on top.
const backdropDefaultAlpha = 0.5

// backdropAlphaMin and backdropAlphaMax clamp opacity so the backdrop can
// never fully vanish (impossible to find again) or fully cover (impossible
// to see what's being traced under it).
const (
	backdropAlphaMin = 0.1
	backdropAlphaMax = 1.0
)

// loadBackdrop prompts for a PNG or JPEG and installs it as the editor's
// trace reference, fit to most of the current view and centered on it.
// Replaces any previously loaded backdrop.
func (g *Game) loadBackdrop() {
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

	g.backdrop = &backdropImage{
		img:     img,
		path:    path,
		x:       cx - iw*scale/2,
		y:       cy + ih*scale/2,
		scale:   scale,
		opacity: backdropDefaultAlpha,
		visible: true,
	}
	g.backdropPosMode = true // land in position mode so it can be aligned right away
	g.sceneErr = ""
}

// clearBackdrop drops the loaded backdrop image.
func (g *Game) clearBackdrop() {
	g.backdrop = nil
	g.backdropPosMode = false
}

// handleBackdropInput drives move (drag) and scale (wheel, anchored at the
// cursor) of the backdrop image while position mode is active. It reports
// whether it consumed the input, so the caller skips camera pan, object
// dragging and the pen tool for that frame.
func (g *Game) handleBackdropInput(mx, my float64, inCanvas bool) bool {
	if !g.backdropPosMode || g.backdrop == nil {
		return false
	}
	_, dy := ebiten.Wheel()
	if dy != 0 && inCanvas {
		g.scaleBackdropAt(mx, my, 1+dy*0.1)
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && inCanvas {
		g.backdropDragging = true
		g.backdropDragLastX, g.backdropDragLastY = mx, my
	}
	if g.backdropDragging && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.backdrop.x += (mx - g.backdropDragLastX) / g.cam.zoom
		g.backdrop.y -= (my - g.backdropDragLastY) / g.cam.zoom
		g.backdropDragLastX, g.backdropDragLastY = mx, my
		return true
	}
	g.backdropDragging = false
	return true
}

// scaleBackdropAt rescales the backdrop image by factor, keeping the world
// point under the cursor fixed -- the same feel as the camera's own zoom.
// (linefire's scaleBackdrop keeps the image's own center fixed instead,
// since its keyboard-driven -/= shortcuts have no cursor position to anchor
// to; kutta's wheel-driven scaling anchors at the cursor like the camera
// zoom it sits alongside.)
func (g *Game) scaleBackdropAt(sx, sy, factor float64) {
	r := g.backdrop
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

// drawBackdrop paints the loaded trace backdrop under the outlines, faded by
// its opacity so a traced line stays legible over it.
func (g *Game) drawBackdrop(vp *ebiten.Image) {
	r := g.backdrop
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
