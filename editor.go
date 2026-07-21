package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"

	ui "github.com/crgimenes/minigui"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"kutta/foil"
	"kutta/scene"
)

// camera maps world (grid) coordinates — x right, y up — to the editor
// viewport's pixels (y down). zoom is pixels per world unit; (ox, oy) is the
// world point shown at the viewport's top-left.
type camera struct {
	ox, oy float64
	zoom   float64
}

func (c camera) worldToScreen(wx, wy float64) (float64, float64) {
	return (wx - c.ox) * c.zoom, (c.oy - wy) * c.zoom
}

func (c camera) screenToWorld(sx, sy float64) (float64, float64) {
	return c.ox + sx/c.zoom, c.oy - sy/c.zoom
}

// pan shifts the view by a screen-space drag delta.
func (c *camera) pan(dsx, dsy float64) {
	c.ox -= dsx / c.zoom
	c.oy += dsy / c.zoom
}

// zoomAt scales by factor while keeping the world point under (sx, sy) fixed.
func (c *camera) zoomAt(sx, sy, factor float64) {
	wx, wy := c.screenToWorld(sx, sy)
	c.zoom *= factor
	if c.zoom < 0.2 {
		c.zoom = 0.2
	}
	if c.zoom > 40 {
		c.zoom = 40
	}
	c.ox = wx - sx/c.zoom
	c.oy = wy + sy/c.zoom
}

// editor colors.
var (
	colEditBg   = color.RGBA{0x0c, 0x0e, 0x14, 0xff}
	colEditGrid = color.RGBA{0x1c, 0x20, 0x2a, 0xff}
	colObj      = color.RGBA{0x9a, 0xa4, 0xb2, 0xff}
	colObjSel   = color.RGBA{0x4c, 0xc6, 0xff, 0xff}
	colVertex   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colRotate   = color.RGBA{0x6f, 0xe0, 0x8a, 0xff} // rotate handle
	colScale    = color.RGBA{0xff, 0xc6, 0x4c, 0xff} // scale handle
	colControl  = color.RGBA{0xff, 0x80, 0xff, 0xff} // Bezier tangent handles
)

// dragKind is what an in-progress canvas drag is manipulating.
type dragKind int

const (
	dragNone   dragKind = iota
	dragPan             // moving the camera
	dragMove            // translating the selected object
	dragRotate          // rotating the selected object about its pivot
	dragScale           // scaling the selected object about its pivot
	dragPivot           // relocating the selected object's pivot
	dragVertex          // moving a single vertex of the selected shape
	dragHandle          // pulling a vertex's Bezier tangent handle
)

// Gizmo handle geometry, in screen pixels.
const (
	rotHandleDist = 42.0 // rotate handle offset above the pivot
	handleHit     = 9.0  // pick radius for a handle
	edgeHit       = 6.0  // pick distance for inserting a vertex on an edge
)

// editMode selects what the editor's gizmo manipulates.
type editMode int

const (
	emGeometry editMode = iota // base shape and pivot (the rest pose)
	emAnimate                  // a keyframed pose at the scrub time
)

// defaultLoop is assigned when entering ANIMATE on a scene with no loop yet, so
// there is a timeline to scrub.
const defaultLoop = 4.0

// toggleEdit switches between the simulator and the editor, sharing the current
// scene. Entering with no scene loaded promotes the interactive foil into a
// one-object scene so there is always something to edit.
func (g *Game) toggleEdit() {
	g.editing = !g.editing
	if !g.editing {
		// Returning to the simulator: push the (possibly edited) geometry to the
		// solver now. A paused scene never re-rasterizes on its own, so without
		// this the flow would keep the pre-edit shape until something else (AoA,
		// the timeline) forced an update. The lift curve is stale after a shape
		// change, so clear it.
		if g.scn != nil {
			g.sim.UpdateSolid(g.sceneMask(g.scn.LoopTime(g.animTime)))
			g.resetCurve()
		}
		return
	}
	if g.scn == nil {
		out := g.placedOutline()
		g.scn = &scene.Scene{Objects: []*scene.Object{{Name: "airfoil", Shape: out, Pivot: centroid(out)}}}
		g.scenePath = "(from foil)"
	}
	g.selObj = -1
	g.selKey = -1
	g.editMode = emGeometry
	g.editTime = 0
	g.scrubbing = false
	g.connectFrom = -1
	// Snap off by default: it gets in the way of fine editing. G turns it on.
	g.snapOn = false
	g.fitCamera()
}

// toggleEditMode flips between GEOMETRY and ANIMATE, giving a static scene a
// default loop so ANIMATE has a timeline.
func (g *Game) toggleEditMode() {
	if g.editMode == emGeometry {
		g.editMode = emAnimate
		if g.scn.Loop <= 0 {
			g.scn.Loop = defaultLoop
		}
		g.editTime = 0
	} else {
		g.editMode = emGeometry
	}
	g.dragK = dragNone
	g.scrubbing = false
}

// activeOutline is the real outline the editor shows and selects against: the
// rest outline in GEOMETRY (smooth when the object is curved), the posed outline
// at the scrub time in ANIMATE. Vertex editing works on the raw knots (o.Shape),
// not this.
func (g *Game) activeOutline(o *scene.Object) []foil.Point {
	if g.editMode == emAnimate {
		return o.PolygonAt(g.editTime)
	}
	return o.Outline()
}

// effPivot is the visible rotation/scale center: the base pivot in GEOMETRY, or
// the pivot shifted by the current pose's translation in ANIMATE.
func (g *Game) effPivot(o *scene.Object) foil.Point {
	if g.editMode == emAnimate {
		p := o.PoseAt(g.editTime)
		return foil.Point{X: o.Pivot.X + p.DX, Y: o.Pivot.Y + p.DY}
	}
	return o.Pivot
}

// fitCamera frames all objects (at their base pose) in the viewport.
func (g *Game) fitCamera() {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, o := range g.scn.Objects {
		for _, p := range o.Shape {
			minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
			maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
		}
	}
	if math.IsInf(minX, 1) {
		g.cam = camera{zoom: pixScale}
		return
	}
	w := math.Max(maxX-minX, 1)
	h := math.Max(maxY-minY, 1)
	const margin = 1.3
	z := math.Min(simW/(w*margin), simH/(h*margin))
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	g.cam = camera{zoom: z, ox: cx - simW/(2*z), oy: cy + simH/(2*z)}
}

// editorInput handles zoom (wheel), camera pan, object selection, the transform
// gizmo, and — in ANIMATE — the timeline scrub and keyframe keys.
func (g *Game) editorInput() {
	g.tick++
	g.runToolbar()   // toolbar (bottom): file/mode actions
	g.runSidePanel() // right panel: object list + rename field
	if !g.editing {
		return // a toolbar button (Simulator) left the editor
	}
	if g.drawing {
		g.drawInput()
		return
	}

	mx, my := g.ptr.pos()
	fmx, fmy := g.ptr.posF()
	inCanvas := mx >= 0 && mx < simW && my >= 0 && my < simH

	// A press in the canvas defocuses the name field so shortcuts resume.
	if inCanvas && g.ptr.pressed {
		g.side.ClearFocus()
	}
	// Clicks on the toolbar row are handled by minigui; ignore them here.
	if my >= simH && my < simH+40 && g.ptr.pressed {
		return
	}

	// Keyboard shortcuts, suppressed while typing into a text field.
	if !g.side.HasFocus() {
		g.editorKeys(fmx, fmy, inCanvas)
	}

	// Timeline scrubbing takes priority over canvas gestures while it is active.
	if g.editMode == emAnimate && g.handleScrub(fmx, fmy) {
		return
	}
	if g.handleBackdropInput(fmx, fmy, inCanvas) {
		return
	}

	_, dy := ebiten.Wheel()
	if dy != 0 && inCanvas {
		g.cam.zoomAt(fmx, fmy, 1+dy*0.1)
	}

	if g.ptr.pressed && inCanvas {
		if g.doubleClickInsert(fmx, fmy) {
			return
		}
		g.beginDrag(fmx, fmy)
	}
	if g.ptr.down {
		g.updateDrag(fmx, fmy)
	}
	if g.ptr.released {
		// A bare click that grabbed nothing selects (or clears) the object.
		if g.dragK == dragNone && !g.dragMoved && inCanvas {
			g.selectAt(fmx, fmy)
		}
		g.dragK = dragNone
	}
}

// animateKeys handles the keyframe, clipboard and loop-length shortcuts in
// ANIMATE mode.
func (g *Game) animateKeys() {
	g.updateSelKey()
	meta := ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight)
	if inpututil.IsKeyJustPressed(ebiten.KeyK) {
		g.setKeyframe()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyX) {
		g.deleteKeyframe()
	}
	if meta && inpututil.IsKeyJustPressed(ebiten.KeyC) {
		g.copyPose()
	}
	if meta && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		g.pastePose()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
		g.loopDelta(0.5)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
		g.loopDelta(-0.5)
	}
}

// setKeyframe stores the selected object's current pose as a keyframe at the
// playhead.
func (g *Game) setKeyframe() {
	if g.selObj < 0 {
		return
	}
	o := g.scn.Objects[g.selObj]
	g.snapshotForUndo()
	o.SetKey(g.editTime, o.PoseAt(g.editTime))
}

// deleteKeyframe removes the selected object's keyframe at the playhead.
func (g *Game) deleteKeyframe() {
	if g.selObj < 0 {
		return
	}
	g.snapshotForUndo()
	g.scn.Objects[g.selObj].DeleteKey(g.editTime)
}

// copyPose copies the pose at the playhead; pastePose writes it as a keyframe
// (works across times and objects, e.g. to mirror a control surface).
func (g *Game) copyPose() {
	if g.selObj < 0 {
		return
	}
	g.poseClip = g.scn.Objects[g.selObj].PoseAt(g.editTime)
	g.poseClipSet = true
}

func (g *Game) pastePose() {
	if g.selObj < 0 || !g.poseClipSet {
		return
	}
	g.snapshotForUndo()
	g.scn.Objects[g.selObj].SetKey(g.editTime, g.poseClip)
}

// loopDelta changes the timeline loop length, keeping the playhead in range.
func (g *Game) loopDelta(d float64) {
	g.scn.Loop = math.Max(0.5, g.scn.Loop+d)
	g.editTime = math.Min(g.editTime, g.scn.Loop)
}

// handleScrub drives the timeline playhead. It returns true when it consumes the
// mouse (a scrub is starting or in progress), so canvas gestures are skipped.
func (g *Game) handleScrub(mx, my float64) bool {
	tx, ty, tw, th := g.timelineRect()
	inStrip := mx >= tx && mx <= tx+tw && my >= ty-8 && my <= ty+th+8
	if g.ptr.pressed && inStrip {
		k := g.keyAtStrip(mx)
		if k >= 0 {
			g.beginKeyDrag(k) // grabbing a keyframe tick retimes it
		} else {
			g.scrubbing = true
		}
	}
	if g.draggingKey {
		if g.ptr.down {
			g.dragKeyTo(mx)
		} else {
			g.endKeyDrag()
		}
		return true
	}
	if !g.scrubbing {
		return false
	}
	if g.ptr.down {
		g.scrubTo(mx)
		return true
	}
	g.scrubbing = false
	return true
}

// keyAtStrip returns the index of the selected object's keyframe whose tick is
// within the pick distance of screen x, or -1.
func (g *Game) keyAtStrip(mx float64) int {
	tx, _, tw, _ := g.timelineRect()
	if g.selObj < 0 || g.scn.Loop <= 0 {
		return -1
	}
	for i, k := range g.scn.Objects[g.selObj].Keys {
		kx := tx + (k.T/g.scn.Loop)*tw
		if math.Abs(kx-mx) <= 6 {
			return i
		}
	}
	return -1
}

// beginKeyDrag lifts keyframe k out of the track so it can be dragged to a new
// time; endKeyDrag drops it back.
func (g *Game) beginKeyDrag(k int) {
	o := g.scn.Objects[g.selObj]
	g.snapshotForUndo()
	g.dragKeyPose = o.Keys[k].Pose
	g.dragKeyT = o.Keys[k].T
	o.DeleteKey(o.Keys[k].T)
	g.draggingKey = true
}

// dragKeyTo moves the lifted keyframe to the time under the cursor.
func (g *Game) dragKeyTo(mx float64) {
	tx, _, tw, _ := g.timelineRect()
	if g.scn.Loop <= 0 {
		return
	}
	t := (mx - tx) / tw * g.scn.Loop
	g.dragKeyT = math.Max(0, math.Min(g.scn.Loop, t))
	g.editTime = g.dragKeyT
}

// endKeyDrag drops the lifted keyframe at its new time.
func (g *Game) endKeyDrag() {
	if g.selObj >= 0 {
		g.scn.Objects[g.selObj].SetKey(g.dragKeyT, g.dragKeyPose)
	}
	g.editTime = g.dragKeyT
	g.draggingKey = false
}

// scrubTo maps a cursor x within the timeline strip to a time in [0, Loop],
// snapping to a nearby keyframe of the selected object so the playhead lands on
// and auto-selects it.
func (g *Game) scrubTo(mx float64) {
	tx, _, tw, _ := g.timelineRect()
	if g.scn.Loop <= 0 {
		return
	}
	t := (mx - tx) / tw * g.scn.Loop
	if g.selObj >= 0 {
		for _, k := range g.scn.Objects[g.selObj].Keys {
			kx := tx + (k.T/g.scn.Loop)*tw
			if math.Abs(kx-mx) <= 6 {
				t = k.T
				break
			}
		}
	}
	g.editTime = math.Max(0, math.Min(g.scn.Loop, t))
}

// updateSelKey selects the keyframe of the selected object that the playhead sits
// on (within a small tolerance), or -1.
func (g *Game) updateSelKey() {
	g.selKey = -1
	if g.selObj < 0 || g.scn.Loop <= 0 {
		return
	}
	tol := g.scn.Loop / 200
	for i, k := range g.scn.Objects[g.selObj].Keys {
		if math.Abs(k.T-g.editTime) <= tol {
			g.selKey = i
			return
		}
	}
}

// timelineRect is the scrub strip in the bottom panel (screen pixels), below the
// toolbar and the animate hint.
func (g *Game) timelineRect() (x, y, w, h float64) {
	return 16, float64(simH) + 88, float64(simW) - 160, 16
}

// dblFrames is the window (in frames, ~0.3s at 60fps) for a double click.
const dblFrames = 18

// doubleClickInsert inserts a vertex when the press is a double click on an edge
// of the selected object. It records every press for the next detection and
// returns true when it consumed the click.
func (g *Game) doubleClickInsert(sx, sy float64) bool {
	dbl := g.tick-g.lastClickTick <= dblFrames && math.Hypot(sx-g.lastClickX, sy-g.lastClickY) <= 5
	g.lastClickTick = g.tick
	g.lastClickX, g.lastClickY = sx, sy
	if !dbl || g.editMode != emGeometry || g.selObj < 0 {
		return false
	}
	return g.insertVertexAt(sx, sy) >= 0
}

// nearLooseEnd returns a gap-bordering vertex within the pick radius, or -1.
func (g *Game) nearLooseEnd(sx, sy float64, o *scene.Object) int {
	for v := range o.Shape {
		if !o.LooseEnd(v) {
			continue
		}
		px, py := g.cam.worldToScreen(o.Shape[v].X, o.Shape[v].Y)
		if math.Hypot(sx-px, sy-py) <= handleHit {
			return v
		}
	}
	return -1
}

// ptArc is one connected piece of an outline: an ordered run of points with their
// symmetric Bezier tangents.
type ptArc struct {
	pts []foil.Point
	h   []foil.Point
}

// connectEnd picks the first loose end, then joins it to the second: connecting
// the two ends of the same arc closes that arc (extracting it as its own object
// when others remain), while connecting ends of different arcs merges them. This
// is how one outline is cut and rejoined into several distinct shapes.
func (g *Game) connectEnd(v int) {
	if g.connectFrom < 0 {
		g.connectFrom = v
		return
	}
	a, b := g.connectFrom, v
	g.connectFrom = -1
	if a == b || g.selObj < 0 {
		return
	}
	o := g.scn.Objects[g.selObj]
	idx := objectArcIdx(o)
	ai, ea := locateEnd(idx, a)
	bi, eb := locateEnd(idx, b)
	if ai < 0 || bi < 0 {
		return
	}
	g.snapshotForUndo()
	if ai == bi {
		g.closeArc(idx, ai)
		return
	}
	mergeArcs(o, idx, ai, ea, bi, eb)
}

// closeArc closes arc ai of the selected object into a loop. When it is the only
// arc the object becomes that closed shape; otherwise the arc is extracted into a
// new object (which is then selected) and the remaining arcs stay in the original.
func (g *Game) closeArc(idx [][]int, ai int) {
	o := g.scn.Objects[g.selObj]
	if len(idx[ai]) < 3 {
		return // too small to be a shape
	}
	arcs := make([]ptArc, len(idx))
	for k := range idx {
		arcs[k] = arcOf(o, idx[k])
	}
	if len(arcs) == 1 {
		o.Shape = arcs[0].pts
		o.Handle = compactHandles(arcs[0].h)
		o.Pivot = centroid(o.Shape)
		o.Gaps = nil
		return
	}
	closed := arcs[ai]
	newObj := &scene.Object{
		Name:   g.nextObjectName(),
		Shape:  closed.pts,
		Handle: compactHandles(closed.h),
		Pivot:  centroid(closed.pts),
	}
	rest := make([]ptArc, 0, len(arcs)-1)
	for k := range arcs {
		if k != ai {
			rest = append(rest, arcs[k])
		}
	}
	rebuildFromArcs(o, rest)
	g.scn.Objects = append(g.scn.Objects, newObj)
	g.selObj = len(g.scn.Objects) - 1 // select the new piece so its controls show
}

// mergeArcs joins arc ai (at end ea) to arc bi (at end eb) into one arc, leaving
// the object broken with one fewer piece.
func mergeArcs(o *scene.Object, idx [][]int, ai, ea, bi, eb int) {
	arcs := make([]ptArc, len(idx))
	for k := range idx {
		arcs[k] = arcOf(o, idx[k])
	}
	first := arcs[ai]
	if ea == 0 { // A must end the first part
		first = reverseArc(first)
	}
	second := arcs[bi]
	if eb != 0 { // B must start the second part
		second = reverseArc(second)
	}
	merged := ptArc{
		pts: append(append([]foil.Point{}, first.pts...), second.pts...),
		h:   append(append([]foil.Point{}, first.h...), second.h...),
	}
	next := make([]ptArc, 0, len(arcs)-1)
	for k := range arcs {
		if k != ai && k != bi {
			next = append(next, arcs[k])
		}
	}
	next = append(next, merged)
	rebuildFromArcs(o, next)
}

// objectArcIdx returns the connected pieces of the outline as lists of Shape
// indices in path order. A closed (unbroken) outline is a single arc.
func objectArcIdx(o *scene.Object) [][]int {
	n := len(o.Shape)
	if len(o.Gaps) != n {
		all := make([]int, n)
		for i := range all {
			all[i] = i
		}
		return [][]int{all}
	}
	start := -1
	for v := range n {
		if o.Gaps[(v-1+n)%n] {
			start = v
			break
		}
	}
	if start < 0 {
		all := make([]int, n)
		for i := range all {
			all[i] = i
		}
		return [][]int{all}
	}
	var arcs [][]int
	v := start
	for consumed := 0; consumed < n; {
		var a []int
		for {
			a = append(a, v)
			consumed++
			if o.Gaps[v] {
				break
			}
			v = (v + 1) % n
		}
		v = (v + 1) % n
		arcs = append(arcs, a)
	}
	return arcs
}

// locateEnd finds the arc for which v is an endpoint, returning the arc index and
// which end (0 = first, 1 = last), or (-1, 0).
func locateEnd(idx [][]int, v int) (int, int) {
	for k, a := range idx {
		if a[0] == v {
			return k, 0
		}
		if a[len(a)-1] == v {
			return k, 1
		}
	}
	return -1, 0
}

// arcOf reads an index list into a ptArc, filling zero tangents when absent.
func arcOf(o *scene.Object, idx []int) ptArc {
	hasH := len(o.Handle) == len(o.Shape)
	a := ptArc{pts: make([]foil.Point, len(idx)), h: make([]foil.Point, len(idx))}
	for i, vi := range idx {
		a.pts[i] = o.Shape[vi]
		if hasH {
			a.h[i] = o.Handle[vi]
		}
	}
	return a
}

// reverseArc reverses point order and mirrors each tangent (out<->in).
func reverseArc(a ptArc) ptArc {
	n := len(a.pts)
	r := ptArc{pts: make([]foil.Point, n), h: make([]foil.Point, n)}
	for i := range a.pts {
		j := n - 1 - i
		r.pts[i] = a.pts[j]
		r.h[i] = foil.Point{X: -a.h[j].X, Y: -a.h[j].Y}
	}
	return r
}

// rebuildFromArcs lays the arcs back into o as a broken outline, with a gap after
// each arc (so the pieces stay separate until reconnected).
func rebuildFromArcs(o *scene.Object, arcs []ptArc) {
	o.Shape = o.Shape[:0]
	var h []foil.Point
	for _, a := range arcs {
		o.Shape = append(o.Shape, a.pts...)
		h = append(h, a.h...)
	}
	n := len(o.Shape)
	o.Gaps = make([]bool, n)
	end := 0
	for _, a := range arcs {
		end += len(a.pts)
		o.Gaps[end-1] = true
	}
	o.Handle = compactHandles(h)
}

// compactHandles returns nil when every tangent is zero, else the slice.
func compactHandles(h []foil.Point) []foil.Point {
	for _, p := range h {
		if p.X != 0 || p.Y != 0 {
			return h
		}
	}
	return nil
}

// beginDrag classifies a fresh press: reconnecting a broken outline, a handle of
// the selected object, a vertex, a body move, or (with Space) a camera pan.
func (g *Game) beginDrag(sx, sy float64) {
	g.dragStartX, g.dragStartY = sx, sy
	g.dragLastX, g.dragLastY = sx, sy
	g.dragMoved = false
	g.dragK = dragNone

	// Pan only while Space is held, so a bare left-drag is free for editing.
	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		g.dragK = dragPan
		return
	}
	if g.selObj < 0 {
		return
	}
	o := g.scn.Objects[g.selObj]
	// Reconnecting a broken outline: click one loose end, then the other.
	if g.editMode == emGeometry && o.Broken() {
		e := g.nearLooseEnd(sx, sy, o)
		if e >= 0 {
			g.connectEnd(e)
			g.dragMoved = true // consume: no select-on-release
			return
		}
	}
	pivotS, rotS, scaleS := g.gizmoHandles(o)
	wx, wy := g.cam.screenToWorld(sx, sy)
	pe := g.effPivot(o)

	vtx := -1
	if g.editMode == emGeometry {
		vtx = g.nearVertex(sx, sy, o)
	}
	// Shift+click on an edge inserts a vertex there and grabs it (a plain click
	// can't be used: a thin body's interior is all near an edge).
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	if g.editMode == emGeometry && vtx < 0 && shift {
		idx := g.insertVertexAt(sx, sy)
		if idx >= 0 {
			g.dragK = dragVertex
			g.dragVtx = idx
			g.dragOrig = append(g.dragOrig[:0], o.Shape...)
			g.dragOrigPivot = o.Pivot
			g.dragWX0, g.dragWY0 = wx, wy
			return
		}
	}
	// Bezier tangent editing (geometry): a handle knob adjusts an existing
	// tangent; Alt+drag on a vertex pulls one out (turning a corner smooth).
	alt := ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
	hIdx, hSign, hOK := -1, 1.0, false
	if g.editMode == emGeometry {
		hIdx, hSign, hOK = g.nearHandle(sx, sy, o)
	}
	switch {
	case math.Hypot(sx-rotS[0], sy-rotS[1]) <= handleHit:
		g.dragK = dragRotate
	case math.Hypot(sx-scaleS[0], sy-scaleS[1]) <= handleHit:
		g.dragK = dragScale
	case g.editMode == emGeometry && math.Hypot(sx-pivotS[0], sy-pivotS[1]) <= handleHit:
		g.dragK = dragPivot
	case hOK:
		g.dragK = dragHandle
		g.dragVtx, g.dragHandleSign = hIdx, hSign
	case g.editMode == emGeometry && alt && vtx >= 0:
		g.dragK = dragHandle
		g.dragVtx, g.dragHandleSign = vtx, 1
	case vtx >= 0:
		g.dragK = dragVertex
		g.dragVtx = vtx
	case pointInPoly(wx, wy, g.activeOutline(o)):
		g.dragK = dragMove
	}
	if g.dragK == dragNone {
		return
	}
	g.snapshotForUndo()
	if g.dragK == dragHandle {
		ensureHandles(o)
	}
	g.dragOrig = append(g.dragOrig[:0], o.Shape...)
	g.dragOrigPivot = o.Pivot
	g.dragPose0 = o.PoseAt(g.editTime)
	g.dragWX0, g.dragWY0 = wx, wy
	g.dragA0 = math.Atan2(wy-pe.Y, wx-pe.X)
	g.dragD0 = math.Hypot(wx-pe.X, wy-pe.Y)
}

// snapPx is the screen-pixel radius within which a point snaps to an existing vertex.
const snapPx = 8.0

// snapPoint snaps a world point to a nearby existing vertex (any object, except
// the one being dragged) or, failing that, to the integer grid. Snapping is off
// when g.snapOn is false, giving free placement.
func (g *Game) snapPoint(wx, wy float64, exObj, exVtx int) (float64, float64) {
	if !g.snapOn {
		return wx, wy
	}
	sx, sy := g.cam.worldToScreen(wx, wy)
	best := snapPx
	bx, by, found := 0.0, 0.0, false
	for oi, o := range g.scn.Objects {
		for vi, p := range o.Shape {
			if oi == exObj && vi == exVtx {
				continue
			}
			px, py := g.cam.worldToScreen(p.X, p.Y)
			d := math.Hypot(sx-px, sy-py)
			if d < best {
				best, bx, by, found = d, p.X, p.Y, true
			}
		}
	}
	if found {
		return bx, by
	}
	return math.Round(wx), math.Round(wy)
}

// ensureHandles makes o.Handle addressable and the same length as o.Shape.
func ensureHandles(o *scene.Object) {
	if len(o.Handle) != len(o.Shape) {
		o.Handle = make([]foil.Point, len(o.Shape))
	}
}

// nearHandle finds a Bezier tangent knob (out at P+H, in at P-H) of the selected
// object within the pick radius, returning the anchor index and the knob sign.
func (g *Game) nearHandle(sx, sy float64, o *scene.Object) (int, float64, bool) {
	if len(o.Handle) != len(o.Shape) {
		return -1, 1, false
	}
	for i, h := range o.Handle {
		if h.X == 0 && h.Y == 0 {
			continue
		}
		ox, oy := g.cam.worldToScreen(o.Shape[i].X+h.X, o.Shape[i].Y+h.Y)
		if math.Hypot(sx-ox, sy-oy) <= handleHit {
			return i, 1, true
		}
		ix, iy := g.cam.worldToScreen(o.Shape[i].X-h.X, o.Shape[i].Y-h.Y)
		if math.Hypot(sx-ix, sy-iy) <= handleHit {
			return i, -1, true
		}
	}
	return -1, 1, false
}

// updateDrag applies the in-progress drag to the camera or the selected object.
func (g *Game) updateDrag(sx, sy float64) {
	if g.dragK == dragNone {
		return
	}
	if math.Hypot(sx-g.dragStartX, sy-g.dragStartY) > 4 {
		g.dragMoved = true
	}
	if g.dragK == dragPan {
		g.cam.pan(sx-g.dragLastX, sy-g.dragLastY)
		g.dragLastX, g.dragLastY = sx, sy
		return
	}
	if g.selObj < 0 {
		return
	}

	o := g.scn.Objects[g.selObj]
	wx, wy := g.cam.screenToWorld(sx, sy)
	if g.editMode == emAnimate {
		g.dragPose(o, wx, wy)
		return
	}
	switch g.dragK {
	case dragMove:
		dx, dy := wx-g.dragWX0, wy-g.dragWY0
		o.Shape = scene.Apply(g.dragOrig, g.dragOrigPivot, scene.Pose{DX: dx, DY: dy, Scale: 1})
		o.Pivot = foil.Point{X: g.dragOrigPivot.X + dx, Y: g.dragOrigPivot.Y + dy}
	case dragRotate:
		ang := math.Atan2(wy-g.dragOrigPivot.Y, wx-g.dragOrigPivot.X) - g.dragA0
		o.Shape = scene.Apply(g.dragOrig, g.dragOrigPivot, scene.Pose{Rot: ang * 180 / math.Pi, Scale: 1})
	case dragScale:
		if g.dragD0 <= 0 {
			return
		}
		f := math.Hypot(wx-g.dragOrigPivot.X, wy-g.dragOrigPivot.Y) / g.dragD0
		if f < 0.05 {
			f = 0.05
		}
		o.Shape = scene.Apply(g.dragOrig, g.dragOrigPivot, scene.Pose{Scale: f})
	case dragPivot:
		o.Pivot = foil.Point{X: g.dragOrigPivot.X + (wx - g.dragWX0), Y: g.dragOrigPivot.Y + (wy - g.dragWY0)}
	case dragVertex:
		if g.dragVtx >= 0 && g.dragVtx < len(o.Shape) && g.dragVtx < len(g.dragOrig) {
			nx := g.dragOrig[g.dragVtx].X + (wx - g.dragWX0)
			ny := g.dragOrig[g.dragVtx].Y + (wy - g.dragWY0)
			nx, ny = g.snapPoint(nx, ny, g.selObj, g.dragVtx)
			o.Shape[g.dragVtx] = foil.Point{X: nx, Y: ny}
		}
	case dragHandle:
		if g.dragVtx >= 0 && g.dragVtx < len(o.Shape) && len(o.Handle) == len(o.Shape) {
			p := o.Shape[g.dragVtx]
			o.Handle[g.dragVtx] = foil.Point{X: g.dragHandleSign * (wx - p.X), Y: g.dragHandleSign * (wy - p.Y)}
		}
	}
}

// segDist returns the clamped projection parameter t in [0,1] and the distance
// from (px,py) to the segment (x0,y0)-(x1,y1).
func segDist(px, py, x0, y0, x1, y1 float64) (float64, float64) {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return 0, math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / l2
	t = math.Max(0, math.Min(1, t))
	cx, cy := x0+t*dx, y0+t*dy
	return t, math.Hypot(px-cx, py-cy)
}

// nearEdge finds the edge of the selected object's outline nearest the cursor
// within edgeHit pixels. It returns the index of the edge's first vertex, the
// world point on that edge, and whether an edge was close enough.
func (g *Game) nearEdge(sx, sy float64, o *scene.Object) (int, foil.Point, bool) {
	poly := o.Shape // edges between knots; insertion adds a knot
	n := len(poly)
	best, bestD := -1, edgeHit
	var bp foil.Point
	for i := range n {
		j := (i + 1) % n
		x0, y0 := g.cam.worldToScreen(poly[i].X, poly[i].Y)
		x1, y1 := g.cam.worldToScreen(poly[j].X, poly[j].Y)
		t, d := segDist(sx, sy, x0, y0, x1, y1)
		if d < bestD {
			best, bestD = i, d
			bp = foil.Point{X: poly[i].X + (poly[j].X-poly[i].X)*t, Y: poly[i].Y + (poly[j].Y-poly[i].Y)*t}
		}
	}
	if best < 0 {
		return 0, foil.Point{}, false
	}
	return best, bp, true
}

// insertVertexAt adds a vertex on the nearest edge of the selected shape at the
// cursor and returns its index, or -1 if no edge is close. Snapshots undo.
func (g *Game) insertVertexAt(sx, sy float64) int {
	if g.selObj < 0 {
		return -1
	}
	o := g.scn.Objects[g.selObj]
	ei, ep, ok := g.nearEdge(sx, sy, o)
	if !ok {
		return -1
	}
	g.snapshotForUndo()
	at := ei + 1
	o.Shape = append(o.Shape, foil.Point{})
	copy(o.Shape[at+1:], o.Shape[at:])
	o.Shape[at] = ep
	return at
}

// nearVertex returns the index of the selected object's vertex within the pick
// radius of the cursor, or -1.
func (g *Game) nearVertex(sx, sy float64, o *scene.Object) int {
	best, bestD := -1, handleHit
	for i, p := range o.Shape {
		px, py := g.cam.worldToScreen(p.X, p.Y)
		d := math.Hypot(sx-px, sy-py)
		if d <= bestD {
			best, bestD = i, d
		}
	}
	return best
}

// deleteVertexAt removes the selected shape's vertex nearest the cursor. On a
// closed outline this breaks the loop open at that point (the two neighbours
// become loose ends to reconnect); on an already-open outline it just drops the
// vertex. It keeps at least a triangle's worth of points.
func (g *Game) deleteVertexAt(sx, sy float64) {
	if g.selObj < 0 {
		return
	}
	o := g.scn.Objects[g.selObj]
	if len(o.Shape) <= 3 {
		return
	}
	i := g.nearVertex(sx, sy, o)
	if i < 0 {
		return
	}
	g.snapshotForUndo()
	deleteVertexBreak(o, i)
}

// deleteVertexBreak removes vertex i and cuts the outline there: the two edges
// that met at i vanish and a single gap is left between its neighbours i-1 and
// i+1. Repeating on another vertex adds another cut, so a loop can be split into
// several pieces; each cut is reconnected independently.
func deleteVertexBreak(o *scene.Object, i int) {
	n := len(o.Shape)
	og := o.Gaps
	if len(og) != n {
		og = make([]bool, n)
	}
	hasH := len(o.Handle) == n
	ns := make([]foil.Point, 0, n-1)
	ng := make([]bool, 0, n-1)
	var nh []foil.Point
	if hasH {
		nh = make([]foil.Point, 0, n-1)
	}
	prev := (i - 1 + n) % n
	for k := range n {
		if k == i {
			continue // drop the vertex and its outgoing edge flag
		}
		ns = append(ns, o.Shape[k])
		if hasH {
			nh = append(nh, o.Handle[k])
		}
		if k == prev {
			ng = append(ng, true) // the merged edge becomes the new gap
		} else {
			ng = append(ng, og[k])
		}
	}
	o.Shape = ns
	o.Handle = nh
	o.Gaps = ng
}

// dragPose updates the selected object's pose at the scrub time from a drag and
// writes it as a keyframe. The visual rotation/scale center is the pivot shifted
// by the pose translation captured at press.
func (g *Game) dragPose(o *scene.Object, wx, wy float64) {
	p := g.dragPose0
	pe := foil.Point{X: o.Pivot.X + g.dragPose0.DX, Y: o.Pivot.Y + g.dragPose0.DY}
	switch g.dragK {
	case dragMove:
		p.DX = g.dragPose0.DX + (wx - g.dragWX0)
		p.DY = g.dragPose0.DY + (wy - g.dragWY0)
	case dragRotate:
		ang := math.Atan2(wy-pe.Y, wx-pe.X) - g.dragA0
		p.Rot = g.dragPose0.Rot + ang*180/math.Pi
	case dragScale:
		if g.dragD0 <= 0 {
			return
		}
		f := math.Hypot(wx-pe.X, wy-pe.Y) / g.dragD0
		if f < 0.05 {
			f = 0.05
		}
		p.Scale = g.dragPose0.Scale * f
	}
	o.SetKey(g.editTime, p)
}

// gizmoHandles returns the screen positions of the pivot, rotate and scale
// handles. The scale knob sits at the outline's top-right on screen and the
// rotate knob just above the outline's top edge (anchored to the shape, not the
// pivot, so it is always on-screen and reachable regardless of where the pivot
// is), both clamped inside the canvas.
func (g *Game) gizmoHandles(o *scene.Object) (pivot, rot, scale [2]float64) {
	pe := g.effPivot(o)
	px, py := g.cam.worldToScreen(pe.X, pe.Y)
	pivot = [2]float64{px, py}

	minSx, minSy := math.Inf(1), math.Inf(1)
	maxSx, maxSy := math.Inf(-1), math.Inf(-1)
	for _, p := range g.activeOutline(o) {
		sx, sy := g.cam.worldToScreen(p.X, p.Y)
		minSx, minSy = math.Min(minSx, sx), math.Min(minSy, sy)
		maxSx, maxSy = math.Max(maxSx, sx), math.Max(maxSy, sy)
	}
	scale = [2]float64{clampCanvasX(maxSx), clampCanvasY(minSy)}
	rx := (minSx + maxSx) / 2
	ry := minSy - rotHandleDist
	if ry < rotHandleDist {
		ry = maxSy + rotHandleDist // flip below the shape if it would clip off the top
	}
	rot = [2]float64{clampCanvasX(rx), clampCanvasY(ry)}
	return
}

func clampCanvasX(x float64) float64 { return math.Max(6, math.Min(simW-6, x)) }
func clampCanvasY(y float64) float64 { return math.Max(6, math.Min(simH-6, y)) }

// snapshotForUndo records the selected object's geometry and keyframes before an
// edit, so a single undo restores either kind of change.
func (g *Game) snapshotForUndo() {
	o := g.scn.Objects[g.selObj]
	g.undoObj = g.selObj
	g.undoShape = append(g.undoShape[:0], o.Shape...)
	g.undoHandle = append(g.undoHandle[:0], o.Handle...)
	g.undoPivot = o.Pivot
	g.undoGaps = append(g.undoGaps[:0], o.Gaps...)
	g.undoKeys = append(g.undoKeys[:0], o.Keys...)
	g.undoValid = true
}

// undoEdit restores the object state captured before the last edit.
func (g *Game) undoEdit() {
	if !g.undoValid || g.undoObj < 0 || g.undoObj >= len(g.scn.Objects) {
		return
	}
	o := g.scn.Objects[g.undoObj]
	o.Shape = append(o.Shape[:0], g.undoShape...)
	o.Handle = append(o.Handle[:0], g.undoHandle...)
	o.Pivot = g.undoPivot
	o.Gaps = append(o.Gaps[:0], g.undoGaps...)
	o.Keys = append(o.Keys[:0], g.undoKeys...)
	g.undoValid = false
}

// autoSmoothSelected gives every vertex a smooth tangent (or clears them all if
// the shape is already smooth), a quick way to round a polygon or reset to
// corners without dragging each handle.
func (g *Game) autoSmoothSelected() {
	if g.selObj < 0 {
		return
	}
	g.snapshotForUndo()
	o := g.scn.Objects[g.selObj]
	if o.HasHandles() {
		o.Handle = nil
		return
	}
	o.AutoSmooth()
}

// selectAt picks the topmost object whose active outline contains the cursor, or
// clears the selection.
func (g *Game) selectAt(sx, sy float64) {
	wx, wy := g.cam.screenToWorld(sx, sy)
	g.selObj = -1
	for i := len(g.scn.Objects) - 1; i >= 0; i-- {
		if pointInPoly(wx, wy, g.activeOutline(g.scn.Objects[i])) {
			g.selObj = i
			return
		}
	}
}

// penHandleMin is the smallest press-to-release drag, in pixels, that counts as
// pulling a Bezier tangent rather than placing a corner.
const penHandleMin = 4.0

// startDraw begins a new pen path in GEOMETRY mode.
func (g *Game) startDraw() {
	g.editMode = emGeometry
	g.drawing = true
	g.penActive = false
	g.draftPts = g.draftPts[:0]
	g.draftH = g.draftH[:0]
}

// drawInput is the pen tool: press places an anchor and the drag until release
// becomes its Bezier tangent (a short drag is a corner). Press near the first
// anchor (or Enter) closes the path; Esc cancels. Wheel still zooms.
func (g *Game) drawInput() {
	mx, my := g.ptr.pos()
	fmx, fmy := g.ptr.posF()
	inCanvas := mx >= 0 && mx < simW && my >= 0 && my < simH

	_, dy := ebiten.Wheel()
	if dy != 0 && inCanvas {
		g.cam.zoomAt(fmx, fmy, 1+dy*0.1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.cancelDraft()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyKPEnter) {
		g.finishDraft()
		return
	}
	if g.ptr.pressed && inCanvas {
		if len(g.draftPts) >= 3 {
			fx, fy := g.cam.worldToScreen(g.draftPts[0].X, g.draftPts[0].Y)
			if math.Hypot(fmx-fx, fmy-fy) <= handleHit {
				g.finishDraft()
				return
			}
		}
		wx, wy := g.cam.screenToWorld(fmx, fmy)
		wx, wy = g.snapPoint(wx, wy, -1, -1)
		g.penAnchor = foil.Point{X: wx, Y: wy}
		g.penActive = true
	}
	if g.penActive && g.ptr.released {
		g.commitPenNode(fmx, fmy)
	}
}

// commitPenNode finishes the anchor under the pen: a near-zero drag is a corner,
// a longer drag sets the anchor's symmetric tangent.
func (g *Game) commitPenNode(sx, sy float64) {
	g.penActive = false
	ax, ay := g.cam.worldToScreen(g.penAnchor.X, g.penAnchor.Y)
	var h foil.Point
	if math.Hypot(sx-ax, sy-ay) >= penHandleMin {
		wx, wy := g.cam.screenToWorld(sx, sy)
		h = foil.Point{X: wx - g.penAnchor.X, Y: wy - g.penAnchor.Y}
	}
	g.draftPts = append(g.draftPts, g.penAnchor)
	g.draftH = append(g.draftH, h)
}

// cancelDraft drops the in-progress path.
func (g *Game) cancelDraft() {
	g.drawing = false
	g.penActive = false
	g.draftPts = g.draftPts[:0]
	g.draftH = g.draftH[:0]
}

// finishDraft turns the draft into a new object (if it has enough anchors),
// selects it, and leaves draw mode.
func (g *Game) finishDraft() {
	if len(g.draftPts) < 3 {
		g.cancelDraft()
		return
	}
	pts := append([]foil.Point(nil), g.draftPts...)
	o := &scene.Object{Name: g.nextObjectName(), Shape: pts, Pivot: centroid(pts)}
	for _, h := range g.draftH {
		if h.X != 0 || h.Y != 0 {
			o.Handle = append([]foil.Point(nil), g.draftH...)
			break
		}
	}
	g.scn.Objects = append(g.scn.Objects, o)
	g.selObj = len(g.scn.Objects) - 1
	g.cancelDraft()
}

// runToolbar drives the editor's clickable bottom toolbar via minigui each
// frame: back to the simulator, file actions, the mode toggle and snap. The
// hotkeys keep working alongside it.
func (g *Game) runToolbar() {
	g.gui.Begin(ui.InputFromEbiten(), 16, float64(simH)+8)
	if g.gui.Button("tb.sim", "Simulator") {
		g.toggleEdit()
	}
	g.gui.SameLine()
	if g.gui.Button("tb.open", "Open") {
		g.editorOpen()
	}
	g.gui.SameLine()
	if g.gui.Button("tb.save", "Save") {
		g.saveScene()
	}
	g.gui.SameLine()
	if g.gui.Button("tb.saveas", "Save As") {
		g.saveSceneAs()
	}
	g.gui.SameLine()
	mode := "To Animate"
	if g.editMode == emAnimate {
		mode = "To Geometry"
	}
	if g.gui.Button("tb.mode", mode) {
		g.toggleEditMode()
	}
	g.gui.SameLine()
	if g.gui.Toggle("tb.snap", "Snap", g.snapOn) {
		g.snapOn = !g.snapOn
	}
	g.gui.SameLine()
	if g.gui.Button("tb.ref", "Reference…") {
		g.loadBackdrop()
	}
	if g.backdrop != nil {
		g.gui.SameLine()
		if g.gui.Toggle("tb.refpos", "Position Ref", g.backdropPosMode) {
			g.backdropPosMode = !g.backdropPosMode
		}
		g.gui.SameLine()
		if g.gui.Toggle("tb.refshow", "Show Ref", g.backdrop.visible) {
			g.backdrop.visible = !g.backdrop.visible
		}
		g.gui.SameLine()
		g.gui.Slider("tb.refop", &g.backdrop.opacity, backdropAlphaMin, backdropAlphaMax)
		g.gui.SameLine()
		if g.gui.Button("tb.refclear", "Clear Ref") {
			g.clearBackdrop()
		}
	}
	g.gui.End()
}

// editorKeys handles the editor's keyboard shortcuts (only when no text field is
// focused).
func (g *Game) editorKeys(fmx, fmy float64, inCanvas bool) {
	meta := ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
	switch {
	case meta && inpututil.IsKeyJustPressed(ebiten.KeyZ):
		g.undoEdit()
	case meta && inpututil.IsKeyJustPressed(ebiten.KeyS) && shift:
		g.saveSceneAs()
	case meta && inpututil.IsKeyJustPressed(ebiten.KeyS):
		g.saveScene()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.startDraw()
	}
	if !meta && inpututil.IsKeyJustPressed(ebiten.KeyC) && g.editMode == emGeometry {
		g.autoSmoothSelected()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyG) {
		g.snapOn = !g.snapOn
	}
	if !meta && inpututil.IsKeyJustPressed(ebiten.KeyO) {
		g.editorOpen()
	}
	// Object clipboard (geometry mode): Cmd+C copy, Cmd+V paste, Cmd+X cut.
	if meta && g.editMode == emGeometry {
		if inpututil.IsKeyJustPressed(ebiten.KeyC) {
			g.copyObject()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyV) {
			g.pasteObject()
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyX) {
			g.cutObject()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		g.toggleEditMode()
	}
	// Delete the vertex under the cursor (geometry mode).
	if g.editMode == emGeometry && inCanvas &&
		(inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyDelete)) {
		g.deleteVertexAt(fmx, fmy)
	}
	if g.editMode == emAnimate {
		g.animateKeys()
	}
}

// objPanelY is the screen y where the object list panel starts, below the info
// rows (which are taller in ANIMATE).
func (g *Game) objPanelY() float64 {
	if g.editMode == emAnimate {
		return 150
	}
	return 110
}

// runSidePanel drives the editor's object list and rename field (minigui): click
// a row to select, edit the name field to rename the selected object.
func (g *Game) runSidePanel() {
	names := make([]string, len(g.scn.Objects))
	for i, o := range g.scn.Objects {
		names[i] = o.Name
	}
	g.side.Begin(ui.InputFromEbiten(), float64(simW)+16, g.objPanelY())
	g.side.Label("OBJECTS")
	g.side.List("obj.list", names, &g.selObj)
	if g.selObj >= 0 && g.selObj < len(g.scn.Objects) {
		g.side.Label("name")
		g.side.TextField("obj.name", &g.scn.Objects[g.selObj].Name)
		o := g.scn.Objects[g.selObj]
		if g.side.Toggle("obj.control", "Control Surface", o.Control) {
			g.setControlObject(o, !o.Control)
		}
	}
	g.side.End()
}

// setControlObject sets whether o is the scene's single live-controlled
// object, clearing the flag on every other object so at most one is active,
// and zeroing the simulator's control slider so a stale deflection from a
// previous control object doesn't carry over.
func (g *Game) setControlObject(o *scene.Object, on bool) {
	o.Control = on
	if !on {
		return
	}
	for _, other := range g.scn.Objects {
		if other != o {
			other.Control = false
		}
	}
	g.controlDeg = 0
}

// editorOpen loads a scene through the native dialog and frames it for editing.
func (g *Game) editorOpen() {
	g.openSceneDialog()
	if g.scn == nil {
		return
	}
	g.selObj = -1
	g.connectFrom = -1
	g.editMode = emGeometry
	g.fitCamera()
}

// pasteOffset shifts a pasted object so it does not sit exactly on the original.
const pasteOffset = 8.0

// cloneObject deep-copies an object so the copy is independent of the original.
func cloneObject(o *scene.Object) *scene.Object {
	return &scene.Object{
		Name:   o.Name,
		Shape:  append([]foil.Point(nil), o.Shape...),
		Handle: append([]foil.Point(nil), o.Handle...),
		Pivot:  o.Pivot,
		Gaps:   append([]bool(nil), o.Gaps...),
		Keys:   append([]scene.Key(nil), o.Keys...),
	}
}

// copyObject puts a copy of the selected object on the clipboard.
func (g *Game) copyObject() {
	if g.selObj < 0 {
		return
	}
	g.objClip = cloneObject(g.scn.Objects[g.selObj])
}

// pasteObject adds a copy of the clipboard object, offset a little and selected.
func (g *Game) pasteObject() {
	if g.objClip == nil {
		return
	}
	n := cloneObject(g.objClip)
	n.Name = g.nextObjectName()
	for i := range n.Shape {
		n.Shape[i].X += pasteOffset
		n.Shape[i].Y += pasteOffset
	}
	n.Pivot.X += pasteOffset
	n.Pivot.Y += pasteOffset
	g.scn.Objects = append(g.scn.Objects, n)
	g.selObj = len(g.scn.Objects) - 1
	g.connectFrom = -1
}

// cutObject copies the selected object to the clipboard and removes it.
func (g *Game) cutObject() {
	if g.selObj < 0 {
		return
	}
	g.objClip = cloneObject(g.scn.Objects[g.selObj])
	g.scn.Objects = append(g.scn.Objects[:g.selObj], g.scn.Objects[g.selObj+1:]...)
	g.selObj = -1
	g.connectFrom = -1
}

// nextObjectName returns the first unused "objectN" name.
func (g *Game) nextObjectName() string {
	used := make(map[string]bool, len(g.scn.Objects))
	for _, o := range g.scn.Objects {
		used[o.Name] = true
	}
	for i := 1; ; i++ {
		n := fmt.Sprintf("object%d", i)
		if !used[n] {
			return n
		}
	}
}

// centroid is the average of the points, used as a sensible default pivot.
func centroid(pts []foil.Point) foil.Point {
	var sx, sy float64
	for _, p := range pts {
		sx, sy = sx+p.X, sy+p.Y
	}
	n := float64(len(pts))
	return foil.Point{X: sx / n, Y: sy / n}
}

func pointInPoly(px, py float64, poly []foil.Point) bool {
	inside := false
	j := len(poly) - 1
	for i := range poly {
		yi, yj := poly[i].Y, poly[j].Y
		if (yi > py) != (yj > py) {
			x := poly[i].X + (py-yi)/(yj-yi)*(poly[j].X-poly[i].X)
			if px < x {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// drawEditor renders the editor canvas and panels.
func (g *Game) drawEditor(screen *ebiten.Image) {
	screen.Fill(colPanel)
	vp := screen.SubImage(image.Rect(0, 0, simW, simH)).(*ebiten.Image)
	vp.Fill(colEditBg)
	g.drawEditGrid(vp)
	g.drawBackdrop(vp)

	for i, o := range g.scn.Objects {
		col := colObj
		if i == g.selObj {
			col = colObjSel
		}
		if g.editMode == emGeometry {
			// Draw each present edge, so cuts (gaps) show as breaks.
			for _, seg := range o.Segments() {
				g.strokePolyline(vp, seg, col)
			}
			continue
		}
		g.strokeWorldPoly(vp, g.activeOutline(o), col, true)
	}
	if g.selObj >= 0 && !g.drawing {
		g.drawGizmo(vp, g.scn.Objects[g.selObj])
	}
	if g.drawing {
		g.drawDraft(vp)
	}
	if !g.drawing {
		g.drawHoverHint(vp)
	}

	vector.StrokeRect(screen, 0, 0, simW, simH, 1, colSep, false)
	g.drawEditorPanels(screen)
	g.side.Render(screen) // object list + rename field
}

// drawDraft renders the in-progress pen path: the curve so far (flattened),
// anchors, the tangent being pulled, the rubber band, and the close target.
func (g *Game) drawDraft(vp *ebiten.Image) {
	// The committed curve, flattened the same way the final object will be.
	if len(g.draftPts) >= 2 {
		flat := scene.OpenOutline(g.draftPts, g.draftH)
		for i := 1; i < len(flat); i++ {
			x0, y0 := g.cam.worldToScreen(flat[i-1].X, flat[i-1].Y)
			x1, y1 := g.cam.worldToScreen(flat[i].X, flat[i].Y)
			vector.StrokeLine(vp, float32(x0), float32(y0), float32(x1), float32(y1), 1.5, colObjSel, true)
		}
	}
	for _, p := range g.draftPts {
		sx, sy := g.cam.worldToScreen(p.X, p.Y)
		vector.FillCircle(vp, float32(sx), float32(sy), 2.5, colVertex, true)
	}
	fmx, fmy := g.ptr.posF()
	// While the button is held, preview the tangent being pulled (both sides).
	if g.penActive {
		ax, ay := g.cam.worldToScreen(g.penAnchor.X, g.penAnchor.Y)
		vector.StrokeLine(vp, float32(ax), float32(ay), float32(fmx), float32(fmy), 1, colControl, true)
		vector.StrokeLine(vp, float32(ax), float32(ay), float32(2*ax-fmx), float32(2*ay-fmy), 1, colControl, true)
		vector.FillCircle(vp, float32(fmx), float32(fmy), 3, colControl, true)
		return
	}
	if len(g.draftPts) == 0 {
		return
	}
	lx, ly := g.cam.worldToScreen(g.draftPts[len(g.draftPts)-1].X, g.draftPts[len(g.draftPts)-1].Y)
	vector.StrokeLine(vp, float32(lx), float32(ly), float32(fmx), float32(fmy), 1, colObj, true)
	if len(g.draftPts) >= 3 {
		fx, fy := g.cam.worldToScreen(g.draftPts[0].X, g.draftPts[0].Y)
		vector.StrokeRect(vp, float32(fx)-4, float32(fy)-4, 8, 8, 1.5, colRotate, true)
	}
}

// drawGizmo draws the selected object's editable knots (GEOMETRY) and the
// move/rotate/scale/pivot handles.
func (g *Game) drawGizmo(vp *ebiten.Image, o *scene.Object) {
	if g.editMode == emGeometry {
		for _, p := range o.Shape {
			sx, sy := g.cam.worldToScreen(p.X, p.Y)
			vector.FillCircle(vp, float32(sx), float32(sy), 2.5, colVertex, true)
		}
		// Bezier tangent handles: a line and knob on each side (out at P+H).
		if len(o.Handle) == len(o.Shape) {
			for i, h := range o.Handle {
				if h.X == 0 && h.Y == 0 {
					continue
				}
				ox, oy := g.cam.worldToScreen(o.Shape[i].X+h.X, o.Shape[i].Y+h.Y)
				ix, iy := g.cam.worldToScreen(o.Shape[i].X-h.X, o.Shape[i].Y-h.Y)
				vector.StrokeLine(vp, float32(ix), float32(iy), float32(ox), float32(oy), 1, colControl, true)
				vector.FillCircle(vp, float32(ox), float32(oy), 3, colControl, true)
				vector.FillCircle(vp, float32(ix), float32(iy), 3, colControl, true)
			}
		}
	}
	// A broken outline: ring every loose end and rubber-band a pending join.
	if g.editMode == emGeometry && o.Broken() {
		for v := range o.Shape {
			if !o.LooseEnd(v) {
				continue
			}
			ex, ey := g.cam.worldToScreen(o.Shape[v].X, o.Shape[v].Y)
			vector.StrokeRect(vp, float32(ex)-4, float32(ey)-4, 8, 8, 1.5, colStall, true)
		}
		if g.connectFrom >= 0 && g.connectFrom < len(o.Shape) {
			cx, cy := g.cam.worldToScreen(o.Shape[g.connectFrom].X, o.Shape[g.connectFrom].Y)
			mx, my := g.ptr.pos()
			vector.StrokeLine(vp, float32(cx), float32(cy), float32(mx), float32(my), 1, colObjSel, true)
		}
	}
	pivotS, rotS, scaleS := g.gizmoHandles(o)
	// Rotate handle: a stem from the pivot to a green knob.
	vector.StrokeLine(vp, float32(pivotS[0]), float32(pivotS[1]), float32(rotS[0]), float32(rotS[1]), 1, colRotate, true)
	vector.FillCircle(vp, float32(rotS[0]), float32(rotS[1]), 4, colRotate, true)
	// Scale handle: an amber square at the shape's far corner.
	vector.FillRect(vp, float32(scaleS[0])-3, float32(scaleS[1])-3, 6, 6, colScale, true)
	// Pivot handle: the CG symbol (drag to relocate the rotation center).
	drawCGSymbol(vp, float32(pivotS[0]), float32(pivotS[1]), 5)
}

// hoverLabel returns a short description of the gizmo element under the cursor,
// or "" when nothing actionable is there. Order matches beginDrag's picking.
func (g *Game) hoverLabel(sx, sy float64) string {
	if g.selObj < 0 {
		return ""
	}
	o := g.scn.Objects[g.selObj]
	pivotS, rotS, scaleS := g.gizmoHandles(o)
	switch {
	case math.Hypot(sx-rotS[0], sy-rotS[1]) <= handleHit:
		return "drag: rotate"
	case math.Hypot(sx-scaleS[0], sy-scaleS[1]) <= handleHit:
		return "drag: scale"
	case g.editMode == emGeometry && math.Hypot(sx-pivotS[0], sy-pivotS[1]) <= handleHit:
		return "drag: move pivot"
	}
	if g.editMode == emGeometry {
		if o.Broken() && g.nearLooseEnd(sx, sy, o) >= 0 {
			return "click two red ends: join"
		}
		_, _, ok := g.nearHandle(sx, sy, o)
		if ok {
			return "drag: bend curve"
		}
		if g.nearVertex(sx, sy, o) >= 0 {
			return "drag: move   Del: cut   Alt+drag: curve"
		}
	}
	wx, wy := g.cam.screenToWorld(sx, sy)
	if pointInPoly(wx, wy, g.activeOutline(o)) {
		return "drag: move object"
	}
	return ""
}

// drawHoverHint shows a tooltip for the gizmo element under the cursor.
func (g *Game) drawHoverHint(vp *ebiten.Image) {
	mx, my := g.ptr.pos()
	if mx < 0 || mx >= simW || my < 0 || my >= simH {
		return
	}
	label := g.hoverLabel(float64(mx), float64(my))
	if label == "" {
		return
	}
	w := float64(len(label))*7 + 10
	x := float64(mx) + 14
	y := float64(my) + 12
	if x+w > simW {
		x = float64(mx) - w - 6
	}
	if y+18 > simH {
		y = float64(my) - 20
	}
	vector.FillRect(vp, float32(x), float32(y), float32(w), 18, colPanel, false)
	vector.StrokeRect(vp, float32(x), float32(y), float32(w), 18, 1, colSep, false)
	drawString(vp, label, x+5, y+3, colValue)
}

// drawEditGrid draws faint world gridlines for orientation.
func (g *Game) drawEditGrid(vp *ebiten.Image) {
	step := 20.0 // world units between lines
	for z := g.cam.zoom; z < 8; z *= 2 {
		step *= 2 // keep lines from getting too dense when zoomed out
	}
	left, top := g.cam.screenToWorld(0, 0)
	right, bottom := g.cam.screenToWorld(simW, simH)
	for x := math.Ceil(left/step) * step; x < right; x += step {
		sx, _ := g.cam.worldToScreen(x, 0)
		vector.StrokeLine(vp, float32(sx), 0, float32(sx), simH, 1, colEditGrid, false)
	}
	for y := math.Ceil(bottom/step) * step; y < top; y += step {
		_, sy := g.cam.worldToScreen(0, y)
		vector.StrokeLine(vp, 0, float32(sy), simW, float32(sy), 1, colEditGrid, false)
	}
}

// strokePolyline strokes an open polyline (no wrap edge).
func (g *Game) strokePolyline(vp *ebiten.Image, poly []foil.Point, col color.Color) {
	for i := 1; i < len(poly); i++ {
		x0, y0 := g.cam.worldToScreen(poly[i-1].X, poly[i-1].Y)
		x1, y1 := g.cam.worldToScreen(poly[i].X, poly[i].Y)
		vector.StrokeLine(vp, float32(x0), float32(y0), float32(x1), float32(y1), 1.5, col, true)
	}
}

func (g *Game) strokeWorldPoly(vp *ebiten.Image, poly []foil.Point, col color.Color, closed bool) {
	n := len(poly)
	last := n // closed: draw the wrap edge n-1 -> 0
	if !closed {
		last = n - 1 // open: leave the gap
	}
	for i := 0; i < last; i++ {
		j := (i + 1) % n
		x0, y0 := g.cam.worldToScreen(poly[i].X, poly[i].Y)
		x1, y1 := g.cam.worldToScreen(poly[j].X, poly[j].Y)
		vector.StrokeLine(vp, float32(x0), float32(y0), float32(x1), float32(y1), 1.5, col, true)
	}
}

func (g *Game) drawEditorPanels(screen *ebiten.Image) {
	x := float64(simW + 16)
	y := 14.0
	y = g.header(screen, "EDITOR", x, y)
	y = g.row(screen, "Source", filepath.Base(g.scenePath), x, y)
	y = g.row(screen, "Objects", fmt.Sprintf("%d", len(g.scn.Objects)), x, y)
	if g.sceneErr != "" {
		y = g.row(screen, "Scene error", g.sceneErr, x, y)
	}

	mode := "GEOMETRY"
	if g.editMode == emAnimate {
		mode = "ANIMATE"
	}
	y = g.rowc(screen, "Mode", mode, x, y, colObjSel)
	if g.editMode == emAnimate {
		y = g.row(screen, "Time", fmt.Sprintf("%.2f / %.1f s", g.editTime, g.scn.Loop), x, y)
		keys := 0
		if g.selObj >= 0 {
			keys = len(g.scn.Objects[g.selObj].Keys)
		}
		g.row(screen, "Keys (sel)", fmt.Sprintf("%d", keys), x, y)
	}

	// The object list + rename field are drawn by the minigui side context
	// (g.side.Render, below); its panel starts at objPanelY().

	top := float64(simH)
	vector.StrokeLine(screen, 0, float32(top), winW, float32(top), 1, colSep, false)
	g.gui.Render(screen) // the minigui toolbar
	if g.drawing {
		drawString(screen, fmt.Sprintf("PEN  %d points   click: corner   click+drag: curve   click the green box (or Enter): close", len(g.draftPts)), 16, top+44, colObjSel)
		drawString(screen, "Esc: cancel   wheel: zoom", 16, top+64, colLabel)
		return
	}
	if g.backdropPosMode && g.backdrop != nil {
		drawString(screen, "REFERENCE  drag: move   wheel: scale about cursor", 16, top+44, colObjSel)
		drawString(screen, "toggle Position Ref off (toolbar) to resume editing", 16, top+64, colLabel)
		return
	}
	if g.editMode == emAnimate {
		drawString(screen, "drag body/green/amber: pose keyframe   K: set   X: del   Cmd+C/V: copy/paste   drag tick: retime   [ ]: loop", 16, top+44, colLabel)
		g.drawTimeline(screen)
		return
	}
	drawString(screen, "P: pen   drag vertex: move   Alt+drag/pink knob: curve   dbl/Shift+click edge: add   Del: cut   join red ends   Cmd+C/V/X: obj", 16, top+44, colLabel)
	drawString(screen, "C: smooth   Cmd+Z: undo   drag empty: pan (or Space)   wheel: zoom", 16, top+64, colLabel)
}

// drawTimeline draws the scrub strip with keyframe ticks for the selected object
// and the current playhead.
func (g *Game) drawTimeline(screen *ebiten.Image) {
	x, y, w, h := g.timelineRect()
	loop := g.scn.Loop
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), colEditBg, false)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, colSep, false)
	if loop <= 0 {
		return
	}
	if g.selObj >= 0 {
		for i, k := range g.scn.Objects[g.selObj].Keys {
			kx := x + (k.T/loop)*w
			col := colScale
			ww := 4.0
			if i == g.selKey { // the keyframe under the playhead: highlight it
				col, ww = colObjSel, 6
			}
			vector.FillRect(screen, float32(kx-ww/2), float32(y)-3, float32(ww), float32(h)+6, col, true)
		}
	}
	if g.draggingKey { // the lifted keyframe, following the cursor
		gx := x + (g.dragKeyT/loop)*w
		vector.FillRect(screen, float32(gx-3), float32(y)-3, 6, float32(h)+6, colObjSel, true)
	}
	phx := x + (g.editTime/loop)*w
	vector.StrokeLine(screen, float32(phx), float32(y)-5, float32(phx), float32(y+h)+5, 1.5, colVertex, true)
	clip := ""
	if g.poseClipSet {
		clip = "  clip set"
	}
	drawString(screen, fmt.Sprintf("t=%.2fs%s", g.editTime, clip), x+w+10, y+2, colValue)
}
