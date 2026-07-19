package main

import (
	ui "github.com/crgimenes/minigui"
)

// pointer is this frame's pointer state for the parts of the app that read the
// mouse directly, outside minigui's widgets: the editor canvas, the timeline
// strip and the reference backdrop.
//
// The position and the press come from minigui, which already folds an active
// touch onto the mouse fields, so a finger drives the editor exactly as a mouse
// does. The release edge is derived here because minigui's Input does not carry
// one; its own widgets never needed it.
type pointer struct {
	x, y     int
	down     bool
	pressed  bool // went down this frame
	released bool // came up this frame

	wasDown bool
}

// sample refreshes the pointer for the current frame. Update calls it once,
// before anything reads it, so every consumer in the frame agrees on the edges.
func (p *pointer) sample() {
	in := ui.InputFromEbiten()
	p.set(int(in.MouseX), int(in.MouseY), in.MouseDown, in.MouseClicked)
}

// set records one frame of pointer state and derives the release edge from the
// previous frame. Kept apart from sample so it can be exercised without a
// running game.
func (p *pointer) set(x, y int, down, pressed bool) {
	p.x, p.y = x, y
	p.down = down
	p.pressed = pressed
	p.released = p.wasDown && !down
	p.wasDown = down
}

// pos returns the pointer position in logical pixels.
func (p *pointer) pos() (int, int) {
	return p.x, p.y
}

// posF returns the pointer position for the geometry code, which works in
// floats throughout.
func (p *pointer) posF() (float64, float64) {
	return float64(p.x), float64(p.y)
}
