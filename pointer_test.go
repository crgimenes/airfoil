package main

import "testing"

// The release edge is the one piece of pointer logic this app owns rather than
// taking from minigui, and a wrong edge means a drag never ends.
func TestPointerReleaseEdge(t *testing.T) {
	var p pointer

	p.set(10, 20, true, true)
	if p.released {
		t.Error("the frame a press starts should not also report a release")
	}
	if !p.pressed || !p.down {
		t.Error("a press should report both down and pressed")
	}

	p.set(12, 20, true, false)
	if p.released {
		t.Error("holding should not report a release")
	}
	if p.pressed {
		t.Error("holding should not keep reporting a press")
	}

	p.set(12, 20, false, false)
	if !p.released {
		t.Error("lifting should report exactly one release")
	}

	p.set(12, 20, false, false)
	if p.released {
		t.Error("the release should last a single frame")
	}
}

// A drag has to keep reporting the moving position while it is held, since the
// editor re-reads it every frame to move the vertex under the pointer.
func TestPointerTracksPositionWhileHeld(t *testing.T) {
	var p pointer

	p.set(10, 20, true, true)
	p.set(48, 64, true, false)

	x, y := p.pos()
	if x != 48 || y != 64 {
		t.Errorf("pos() = (%d, %d), want the moved position (48, 64)", x, y)
	}
	fx, fy := p.posF()
	if fx != 48 || fy != 64 {
		t.Errorf("posF() = (%v, %v), want (48, 64)", fx, fy)
	}
}

// A press with no prior frame must not look like a release, which would fire
// selectAt on the very first tap after the editor opens.
func TestPointerFirstFrameIsNotARelease(t *testing.T) {
	var p pointer

	p.set(5, 5, false, false)
	if p.released {
		t.Error("an idle first frame should not report a release")
	}
}
