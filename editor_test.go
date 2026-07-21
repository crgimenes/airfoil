package main

import (
	"math"
	"testing"

	"kutta/foil"
	"kutta/scene"
)

func capprox(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCameraRoundTrip(t *testing.T) {
	c := camera{ox: 10, oy: 50, zoom: 3}
	for _, p := range [][2]float64{{0, 0}, {123, 45}, {-7, 99}} {
		sx, sy := c.worldToScreen(p[0], p[1])
		wx, wy := c.screenToWorld(sx, sy)
		if !capprox(wx, p[0]) || !capprox(wy, p[1]) {
			t.Errorf("round trip %v -> screen(%g,%g) -> world(%g,%g)", p, sx, sy, wx, wy)
		}
	}
}

func TestCameraYIsFlipped(t *testing.T) {
	c := camera{ox: 0, oy: 0, zoom: 1}
	// Higher world y must map to a smaller screen y (screen y points down).
	_, syLow := c.worldToScreen(0, 1)
	_, syHigh := c.worldToScreen(0, 10)
	if !(syHigh < syLow) {
		t.Errorf("y flip wrong: y=1 -> %g, y=10 -> %g", syLow, syHigh)
	}
}

func TestCameraZoomAtKeepsCursorFixed(t *testing.T) {
	c := camera{ox: 5, oy: 80, zoom: 2}
	const sx, sy = 240, 120
	wxBefore, wyBefore := c.screenToWorld(sx, sy)
	c.zoomAt(sx, sy, 1.5)
	wxAfter, wyAfter := c.screenToWorld(sx, sy)
	if !capprox(wxBefore, wxAfter) || !capprox(wyBefore, wyAfter) {
		t.Errorf("zoomAt moved the cursor point: (%g,%g) -> (%g,%g)", wxBefore, wyBefore, wxAfter, wyAfter)
	}
	if !capprox(c.zoom, 3) {
		t.Errorf("zoom = %g, want 3", c.zoom)
	}
}

func TestCameraZoomClamps(t *testing.T) {
	c := camera{zoom: 1}
	for range 20 {
		c.zoomAt(0, 0, 2)
	}
	if c.zoom > 40 {
		t.Errorf("zoom not clamped at the top: %g", c.zoom)
	}
	for range 20 {
		c.zoomAt(0, 0, 0.5)
	}
	if c.zoom < 0.2 {
		t.Errorf("zoom not clamped at the bottom: %g", c.zoom)
	}
}

// editGame builds a minimal Game (no solver) with one square object, for
// exercising the editor's gizmo input directly.
func editGame() *Game {
	sq := []foil.Point{{X: 2, Y: 2}, {X: 6, Y: 2}, {X: 6, Y: 6}, {X: 2, Y: 6}}
	g := &Game{selObj: 0, undoObj: -1, connectFrom: -1}
	g.cam = camera{ox: 0, oy: 0, zoom: 10}
	g.scn = &scene.Scene{Objects: []*scene.Object{{
		Name:  "sq",
		Shape: sq,
		Pivot: foil.Point{X: 4, Y: 4},
	}}}
	return g
}

func TestGizmoMoveAndUndo(t *testing.T) {
	g := editGame()
	// Press inside the square (world 3,3 -> screen 30,-30), away from handles.
	g.beginDrag(30, -30)
	if g.dragK != dragMove {
		t.Fatalf("dragK = %d, want dragMove", g.dragK)
	}
	// Drag +5 world units in x (screen +50px at zoom 10).
	g.updateDrag(80, -30)
	o := g.scn.Objects[0]
	if !capprox(o.Shape[0].X, 7) || !capprox(o.Shape[0].Y, 2) {
		t.Errorf("moved corner = (%g,%g), want (7,2)", o.Shape[0].X, o.Shape[0].Y)
	}
	if !capprox(o.Pivot.X, 9) || !capprox(o.Pivot.Y, 4) {
		t.Errorf("moved pivot = (%g,%g), want (9,4)", o.Pivot.X, o.Pivot.Y)
	}
	g.undoEdit()
	if !capprox(o.Shape[0].X, 2) || !capprox(o.Pivot.X, 4) {
		t.Errorf("undo did not restore: corner.X=%g pivot.X=%g", o.Shape[0].X, o.Pivot.X)
	}
}

func TestAnimateMoveCreatesKeyframe(t *testing.T) {
	g := editGame()
	g.editMode = emAnimate
	g.scn.Loop = 4
	g.editTime = 1
	// Drag the body +5 world x (screen 30,-30 -> 80,-30 at zoom 10).
	g.beginDrag(30, -30)
	if g.dragK != dragMove {
		t.Fatalf("dragK = %d, want dragMove", g.dragK)
	}
	g.updateDrag(80, -30)
	o := g.scn.Objects[0]
	if len(o.Keys) != 1 {
		t.Fatalf("keys = %d, want 1", len(o.Keys))
	}
	if !capprox(o.Keys[0].T, 1) {
		t.Errorf("key time = %g, want 1", o.Keys[0].T)
	}
	if !capprox(o.Keys[0].Pose.DX, 5) || !capprox(o.Keys[0].Pose.DY, 0) {
		t.Errorf("pose = %+v, want DX=5 DY=0", o.Keys[0].Pose)
	}
	// The base shape must be untouched (animation rides on top of geometry).
	if !capprox(o.Shape[0].X, 2) {
		t.Errorf("base shape moved: corner.X = %g, want 2", o.Shape[0].X)
	}
}

func TestAnimateUndoRestoresKeys(t *testing.T) {
	g := editGame()
	g.editMode = emAnimate
	g.scn.Loop = 4
	g.editTime = 2
	g.beginDrag(30, -30)
	g.updateDrag(80, -30)
	if len(g.scn.Objects[0].Keys) != 1 {
		t.Fatalf("expected a keyframe before undo")
	}
	g.undoEdit()
	if len(g.scn.Objects[0].Keys) != 0 {
		t.Errorf("undo did not remove the keyframe: %v", g.scn.Objects[0].Keys)
	}
}

func TestToggleEditModeSetsLoop(t *testing.T) {
	g := editGame()
	g.scn.Loop = 0
	g.toggleEditMode()
	if g.editMode != emAnimate {
		t.Fatalf("mode = %d, want emAnimate", g.editMode)
	}
	if !capprox(g.scn.Loop, defaultLoop) {
		t.Errorf("loop = %g, want %g", g.scn.Loop, defaultLoop)
	}
}

func TestScrubSnapsToKeyframe(t *testing.T) {
	g := editGame()
	g.scn.Loop = 4
	o := g.scn.Objects[0]
	o.SetKey(2, scene.Pose{Rot: -20, Scale: 1})
	tx, _, tw, _ := g.timelineRect()
	kx := tx + (2.0/4.0)*tw
	g.scrubTo(kx + 3) // a few px off the key -> snaps onto it
	if !capprox(g.editTime, 2) {
		t.Errorf("scrub near key -> editTime=%g, want 2", g.editTime)
	}
	g.updateSelKey()
	if g.selKey != 0 {
		t.Errorf("selKey = %d, want 0 (key under playhead)", g.selKey)
	}
	g.scrubTo(tx) // far from the key
	g.updateSelKey()
	if g.selKey != -1 {
		t.Errorf("selKey = %d, want -1 away from any key", g.selKey)
	}
}

func TestKeyframeRetimeDrag(t *testing.T) {
	g := editGame()
	g.scn.Loop = 4
	o := g.scn.Objects[0]
	o.SetKey(1, scene.Pose{Rot: -30, Scale: 1})
	tx, _, tw, _ := g.timelineRect()
	// Grab the tick at t=1 and drag it to t=3.
	k := g.keyAtStrip(tx + (1.0/4.0)*tw)
	if k != 0 {
		t.Fatalf("keyAtStrip on the tick = %d, want 0", k)
	}
	g.beginKeyDrag(0)
	if !g.draggingKey || len(o.Keys) != 0 {
		t.Fatalf("begin drag: dragging=%v keys=%d (should lift the key out)", g.draggingKey, len(o.Keys))
	}
	g.dragKeyTo(tx + (3.0/4.0)*tw)
	g.endKeyDrag()
	if g.draggingKey || len(o.Keys) != 1 {
		t.Fatalf("end drag: dragging=%v keys=%d", g.draggingKey, len(o.Keys))
	}
	if !capprox(o.Keys[0].T, 3) {
		t.Errorf("retimed key T = %g, want 3", o.Keys[0].T)
	}
	if !capprox(o.Keys[0].Pose.Rot, -30) {
		t.Errorf("retime lost the pose: Rot=%g, want -30", o.Keys[0].Pose.Rot)
	}
}

func TestScrubToMapsTime(t *testing.T) {
	g := editGame()
	g.scn.Loop = 4
	tx, _, tw, _ := g.timelineRect()
	g.scrubTo(tx + tw/2)
	if !capprox(g.editTime, 2) {
		t.Errorf("editTime = %g, want 2", g.editTime)
	}
	g.scrubTo(tx - 1000) // clamp low
	if g.editTime != 0 {
		t.Errorf("editTime = %g, want clamp to 0", g.editTime)
	}
}

func TestGizmoPivotHandleSelected(t *testing.T) {
	g := editGame()
	// Press on the pivot handle: pivot at world (4,4) -> screen (40,-40).
	g.beginDrag(40, -40)
	if g.dragK != dragPivot {
		t.Fatalf("dragK = %d, want dragPivot", g.dragK)
	}
	// Relocate the pivot without moving geometry.
	g.updateDrag(50, -40) // +1 world unit x
	o := g.scn.Objects[0]
	if !capprox(o.Pivot.X, 5) {
		t.Errorf("pivot.X = %g, want 5", o.Pivot.X)
	}
	if !capprox(o.Shape[0].X, 2) {
		t.Errorf("geometry moved with pivot: corner.X = %g, want 2", o.Shape[0].X)
	}
}

func TestCentroid(t *testing.T) {
	sq := []foil.Point{{X: 2, Y: 2}, {X: 6, Y: 2}, {X: 6, Y: 6}, {X: 2, Y: 6}}
	c := centroid(sq)
	if !capprox(c.X, 4) || !capprox(c.Y, 4) {
		t.Errorf("centroid = (%g,%g), want (4,4)", c.X, c.Y)
	}
}

func TestNextObjectName(t *testing.T) {
	g := editGame() // one object named "sq"
	n := g.nextObjectName()
	if n != "object1" {
		t.Errorf("name = %q, want object1", n)
	}
	g.scn.Objects[0].Name = "object1"
	n = g.nextObjectName()
	if n != "object2" {
		t.Errorf("name = %q, want object2 when object1 is taken", n)
	}
}

func TestFinishDraft(t *testing.T) {
	g := editGame()
	g.drawing = true
	g.draftPts = []foil.Point{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 2, Y: 4}}
	g.finishDraft()
	if g.drawing {
		t.Error("still drawing after finish")
	}
	if len(g.scn.Objects) != 2 {
		t.Fatalf("objects = %d, want 2", len(g.scn.Objects))
	}
	o := g.scn.Objects[1]
	if g.selObj != 1 {
		t.Errorf("selObj = %d, want 1 (new object selected)", g.selObj)
	}
	if len(o.Shape) != 3 || o.Name != "object1" {
		t.Errorf("new object: name=%q pts=%d", o.Name, len(o.Shape))
	}
	if !capprox(o.Pivot.X, 2) {
		t.Errorf("pivot.X = %g, want 2 (centroid)", o.Pivot.X)
	}
}

func TestFinishDraftRejectsDegenerate(t *testing.T) {
	g := editGame()
	g.drawing = true
	g.draftPts = []foil.Point{{X: 0, Y: 0}, {X: 4, Y: 0}} // only 2 points
	g.finishDraft()
	if len(g.scn.Objects) != 1 {
		t.Errorf("objects = %d, want 1 (degenerate draft rejected)", len(g.scn.Objects))
	}
	if g.drawing {
		t.Error("should leave draw mode even on rejection")
	}
}

func TestSnapPoint(t *testing.T) {
	g := editGame() // square corners (2,2)(6,2)(6,6)(2,6), zoom 10
	g.snapOn = true
	// Far from any vertex -> snaps to the integer grid.
	x, y := g.snapPoint(3.4, 5.6, -1, -1)
	if !capprox(x, 3) || !capprox(y, 6) {
		t.Errorf("grid snap = (%g,%g), want (3,6)", x, y)
	}
	// Near an existing vertex -> snaps onto it exactly.
	x, y = g.snapPoint(6.2, 6.1, -1, -1)
	if !capprox(x, 6) || !capprox(y, 6) {
		t.Errorf("vertex snap = (%g,%g), want (6,6)", x, y)
	}
	// Snap off -> exact position.
	g.snapOn = false
	x, y = g.snapPoint(3.4, 5.6, -1, -1)
	if !capprox(x, 3.4) || !capprox(y, 5.6) {
		t.Errorf("snap off = (%g,%g), want (3.4,5.6)", x, y)
	}
}

func TestObjectCopyPaste(t *testing.T) {
	g := editGame()
	g.scn.Objects[0].Keys = []scene.Key{{T: 1, Pose: scene.Pose{Rot: -20, Scale: 1}}}
	g.copyObject()
	g.pasteObject()
	if len(g.scn.Objects) != 2 {
		t.Fatalf("objects = %d, want 2 after paste", len(g.scn.Objects))
	}
	if g.selObj != 1 {
		t.Errorf("selObj = %d, want 1 (paste selects the copy)", g.selObj)
	}
	nw := g.scn.Objects[1]
	// The paste is an independent, offset copy that carries the keyframes.
	if !capprox(nw.Shape[0].X, 2+pasteOffset) {
		t.Errorf("paste not offset: X=%g, want %g", nw.Shape[0].X, 2+pasteOffset)
	}
	if len(nw.Keys) != 1 {
		t.Errorf("paste lost keyframes: %d", len(nw.Keys))
	}
	nw.Shape[0].X = 999 // mutating the copy must not touch the original
	if capprox(g.scn.Objects[0].Shape[0].X, 999) {
		t.Error("paste shares the shape slice with the original")
	}
}

func TestObjectCut(t *testing.T) {
	g := editGame()
	g.copyObject()
	g.pasteObject() // now 2 objects, selObj=1
	g.cutObject()   // remove object 1
	if len(g.scn.Objects) != 1 {
		t.Fatalf("objects = %d, want 1 after cut", len(g.scn.Objects))
	}
	if g.selObj != -1 {
		t.Errorf("selObj = %d, want -1 after cut", g.selObj)
	}
	// The cut object is on the clipboard and can be pasted back.
	g.pasteObject()
	if len(g.scn.Objects) != 2 {
		t.Errorf("objects = %d, want 2 after paste-back", len(g.scn.Objects))
	}
}

func TestHoverLabel(t *testing.T) {
	g := editGame() // square, zoom 10; geometry mode, object 0 selected
	_, rotS, _ := g.gizmoHandles(g.scn.Objects[0])
	got := g.hoverLabel(rotS[0], rotS[1])
	if got != "drag: rotate" {
		t.Errorf("over rotate knob = %q, want \"drag: rotate\"", got)
	}
	got = g.hoverLabel(20, -20)
	if got == "" || got == "drag: move object" {
		t.Errorf("over vertex 0 = %q, want a vertex hint", got)
	}
	got = g.hoverLabel(30, -30)
	if got != "drag: move object" {
		t.Errorf("inside body = %q, want \"drag: move object\"", got)
	}
	got = g.hoverLabel(500, 500)
	if got != "" {
		t.Errorf("over empty space = %q, want empty", got)
	}
}

func TestNearVertex(t *testing.T) {
	g := editGame() // square, zoom 10, origin at world (0,0)
	o := g.scn.Objects[0]
	// Vertex (2,2) -> screen (20,-20).
	i := g.nearVertex(20, -20, o)
	if i != 0 {
		t.Errorf("nearVertex at corner 0 = %d, want 0", i)
	}
	i = g.nearVertex(200, 200, o)
	if i != -1 {
		t.Errorf("nearVertex far away = %d, want -1", i)
	}
}

func TestDragVertexMovesPoint(t *testing.T) {
	g := editGame()
	// Grab vertex 3 at (2,6) -> screen (20,-60); away from the gizmo handles.
	g.beginDrag(20, -60)
	if g.dragK != dragVertex || g.dragVtx != 3 {
		t.Fatalf("dragK=%d dragVtx=%d, want dragVertex idx 3", g.dragK, g.dragVtx)
	}
	g.updateDrag(70, -60) // +5 world x
	o := g.scn.Objects[0]
	if !capprox(o.Shape[3].X, 7) || !capprox(o.Shape[3].Y, 6) {
		t.Errorf("vertex 3 = (%g,%g), want (7,6)", o.Shape[3].X, o.Shape[3].Y)
	}
	// Other vertices untouched.
	if !capprox(o.Shape[0].X, 2) {
		t.Errorf("vertex 0 moved: X=%g, want 2", o.Shape[0].X)
	}
}

func TestDeleteVertexBreaksAndReconnects(t *testing.T) {
	g := editGame()
	o := g.scn.Objects[0]
	g.deleteVertexAt(20, -60) // near vertex 3 -> cut the closed loop
	if len(o.Shape) != 3 {
		t.Fatalf("after delete: %d verts, want 3", len(o.Shape))
	}
	if !o.Broken() {
		t.Error("delete should cut the outline")
	}
	// Reconnect: pick both loose ends of the gap to close.
	gi := -1
	for i, gap := range o.Gaps {
		if gap {
			gi = i
		}
	}
	g.connectEnd(gi)
	if g.connectFrom != gi {
		t.Fatalf("connectFrom = %d, want %d after first end", g.connectFrom, gi)
	}
	g.connectEnd((gi + 1) % len(o.Shape))
	if o.Broken() || g.connectFrom != -1 {
		t.Errorf("reconnect failed: broken=%v connectFrom=%d", o.Broken(), g.connectFrom)
	}
}

// splitGame is a hexagon already cut into two arcs [0,1,2] and [3,4,5].
func splitGame() *Game {
	g := editGame()
	o := g.scn.Objects[0]
	o.Shape = []foil.Point{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 2, Y: 4}, {X: 0, Y: 4}}
	o.Gaps = []bool{false, false, true, false, false, true}
	return g
}

func TestConnectClosesArcIntoNewObject(t *testing.T) {
	g := splitGame()
	remnant := g.scn.Objects[0]
	g.connectEnd(0) // arc [0,1,2] ends are 0 and 2
	g.connectEnd(2)
	if len(g.scn.Objects) != 2 {
		t.Fatalf("objects = %d, want 2 (arc extracted)", len(g.scn.Objects))
	}
	if len(remnant.Shape) != 3 || !remnant.Broken() {
		t.Errorf("remnant: shape=%d broken=%v, want 3 & broken", len(remnant.Shape), remnant.Broken())
	}
	nw := g.scn.Objects[1]
	if len(nw.Shape) != 3 || nw.Broken() {
		t.Errorf("new obj: shape=%d broken=%v, want 3 & closed", len(nw.Shape), nw.Broken())
	}
	if g.selObj != 1 {
		t.Errorf("selObj = %d, want 1 (new piece auto-selected)", g.selObj)
	}
	// Reselect the remnant and close it too -> two distinct solids.
	g.selObj = 0
	g.connectEnd(0)
	g.connectEnd(2)
	if remnant.Broken() {
		t.Error("remnant should be closed after joining its ends")
	}
}

func TestConnectMergesDifferentArcs(t *testing.T) {
	g := splitGame()
	o := g.scn.Objects[0]
	g.connectEnd(0) // end of arc [0,1,2]
	g.connectEnd(3) // end of arc [3,4,5]
	if len(g.scn.Objects) != 1 {
		t.Fatalf("objects = %d, want 1 (merge, not split)", len(g.scn.Objects))
	}
	if len(o.Shape) != 6 {
		t.Errorf("merged shape = %d, want 6", len(o.Shape))
	}
	arcs := objectArcIdx(o)
	if len(arcs) != 1 {
		t.Errorf("arcs after merge = %d, want 1", len(arcs))
	}
}

func TestDeleteTwiceKeepsTwoGaps(t *testing.T) {
	// A pentagon can be cut twice (into two pieces) and stay >= a triangle.
	g := editGame()
	o := g.scn.Objects[0]
	o.Shape = []foil.Point{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 6, Y: 3}, {X: 3, Y: 6}, {X: 0, Y: 4}}
	deleteVertexBreak(o, 1)
	deleteVertexBreak(o, 2)
	gaps := 0
	for _, gp := range o.Gaps {
		if gp {
			gaps++
		}
	}
	if gaps != 2 {
		t.Errorf("gaps after two deletes = %d, want 2", gaps)
	}
	if !o.Broken() {
		t.Error("should still be broken")
	}
}

func TestNearEdge(t *testing.T) {
	g := editGame()
	o := g.scn.Objects[0]
	// Midpoint of edge 0 (2,2)-(6,2) is (4,2) -> screen (40,-20).
	ei, p, ok := g.nearEdge(40, -20, o)
	if !ok || ei != 0 {
		t.Fatalf("nearEdge = (%d,%v,%v), want edge 0", ei, p, ok)
	}
	if !capprox(p.X, 4) || !capprox(p.Y, 2) {
		t.Errorf("edge point = (%g,%g), want (4,2)", p.X, p.Y)
	}
	_, _, ok = g.nearEdge(200, 200, o)
	if ok {
		t.Error("nearEdge far away returned ok")
	}
}

func TestInsertVertexAt(t *testing.T) {
	g := editGame()
	idx := g.insertVertexAt(40, -20) // on edge 0
	if idx != 1 {
		t.Fatalf("insert index = %d, want 1", idx)
	}
	o := g.scn.Objects[0]
	if len(o.Shape) != 5 {
		t.Fatalf("verts = %d, want 5", len(o.Shape))
	}
	// The new vertex sits between the two original edge-0 endpoints.
	if !capprox(o.Shape[1].X, 4) || !capprox(o.Shape[1].Y, 2) {
		t.Errorf("inserted vertex = (%g,%g), want (4,2)", o.Shape[1].X, o.Shape[1].Y)
	}
	if !capprox(o.Shape[0].X, 2) || !capprox(o.Shape[2].X, 6) {
		t.Errorf("neighbors reordered: %v", o.Shape)
	}
	// Undo removes the inserted vertex.
	g.undoEdit()
	if len(g.scn.Objects[0].Shape) != 4 {
		t.Errorf("after undo: %d verts, want 4", len(g.scn.Objects[0].Shape))
	}
}

func TestAutoSmoothSelectedAndUndo(t *testing.T) {
	g := editGame()
	o := g.scn.Objects[0]
	if o.HasHandles() {
		t.Fatal("new object should have no handles")
	}
	g.autoSmoothSelected()
	if !o.HasHandles() {
		t.Error("autoSmoothSelected did not create handles")
	}
	g.undoEdit()
	if o.HasHandles() {
		t.Error("undo did not restore no-handles state")
	}
	// A second smooth followed by another smooth clears (toggle behavior).
	g.autoSmoothSelected()
	g.autoSmoothSelected()
	if o.HasHandles() {
		t.Error("smoothing twice should clear handles")
	}
}

func TestDragHandlePullsTangent(t *testing.T) {
	g := editGame() // square, zoom 10, origin at world (0,0)
	// Alt-drag vertex 3 at (2,6) -> screen (20,-60) to pull a tangent.
	// beginDrag reads Alt via ebiten; instead exercise the math directly.
	o := g.scn.Objects[0]
	ensureHandles(o)
	g.dragK = dragHandle
	g.dragVtx = 3
	g.dragHandleSign = 1
	g.selObj = 0
	g.updateDrag(90, -60) // world (9,6); anchor (2,6) -> handle (7,0)
	if !capprox(o.Handle[3].X, 7) || !capprox(o.Handle[3].Y, 0) {
		t.Errorf("handle = %+v, want (7,0)", o.Handle[3])
	}
}

func TestSceneBodyCenter(t *testing.T) {
	g := editGame() // square (2..6), alphaDeg 0, animTime 0
	cx, cy := g.sceneBodyCenter()
	if !capprox(cx, 4) || !capprox(cy, 4) {
		t.Errorf("body center = (%g,%g), want (4,4)", cx, cy)
	}
}

func TestPointInPoly(t *testing.T) {
	sq := []foil.Point{{X: 2, Y: 2}, {X: 6, Y: 2}, {X: 6, Y: 6}, {X: 2, Y: 6}}
	if !pointInPoly(4, 4, sq) {
		t.Error("center should be inside")
	}
	if pointInPoly(0, 0, sq) {
		t.Error("outside point reported inside")
	}
}

// TestJoinChainsPicksClosestOrientation checks that two open chains join
// end-to-end regardless of which direction each was drawn in: b reversed
// relative to a should still produce one continuous loop, not a crossed one.
func TestJoinChainsPicksClosestOrientation(t *testing.T) {
	// a: (0,0) -> (10,0), the top surface, LE to TE.
	a := []foil.Point{{X: 0, Y: 0}, {X: 5, Y: 1}, {X: 10, Y: 0}}
	// b drawn TE -> LE (the natural way to trace a bottom surface back), so it
	// already abuts a's end without needing a reversal.
	b := []foil.Point{{X: 10, Y: 0}, {X: 5, Y: -1}, {X: 0, Y: 0}}
	got := joinChains(a, b)
	if len(got) != len(a)+len(b) {
		t.Fatalf("got %d points, want %d", len(got), len(a)+len(b))
	}
	if got[0] != a[0] || got[len(a)] != b[0] {
		t.Errorf("expected a then b unreversed, got %+v", got)
	}

	// Same two shapes, but b traced the other way (LE -> TE): joinChains should
	// reverse it internally so the result is still one continuous loop.
	bRev := reversePoints(b)
	got2 := joinChains(a, bRev)
	if got2[len(a)] != b[0] {
		t.Errorf("expected joinChains to reverse b back to TE-first, got seam point %+v", got2[len(a)])
	}
}
