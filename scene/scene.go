// Package scene models an animated 2D scene: a set of closed-outline objects,
// each carrying a keyframed pose track, that the host resolves at a given time
// into concrete polygons and a union solid mask for the solver.
//
// It is rendering-free and carries no airfoil-specific logic beyond using
// foil.Point and foil.Rasterize, so it drives the wind tunnel today (a wing plus
// animated control surfaces) and can be reused later (e.g. animated obstacles).
package scene

import (
	"math"
	"sort"

	"kutta/foil"
)

// Pose places an object relative to its base shape: a rotation and uniform
// scale about the object's pivot, then a translation. Coordinates and the
// translation are in grid cells; Rot is degrees, counter-clockwise with y up.
type Pose struct {
	DX, DY float64
	Rot    float64
	Scale  float64
}

// Identity is the no-op pose (unit scale, no move, no rotation).
func Identity() Pose { return Pose{Scale: 1} }

func lerp(a, b, u float64) float64 { return a + (b-a)*u }

// blend linearly interpolates between two poses.
func blend(a, b Pose, u float64) Pose {
	return Pose{
		DX:    lerp(a.DX, b.DX, u),
		DY:    lerp(a.DY, b.DY, u),
		Rot:   lerp(a.Rot, b.Rot, u),
		Scale: lerp(a.Scale, b.Scale, u),
	}
}

// Key is a pose held at a point in time, in seconds.
type Key struct {
	T    float64
	Pose Pose
}

// Object is a closed outline with an optional keyframe track. Shape and Pivot
// are in the coordinate space the host rasterizes in (grid cells).
//
// Handle gives each anchor an optional Bezier tangent (the pen-tool model): when
// len(Handle)==len(Shape), Handle[i] is anchor i's symmetric out-tangent offset,
// and the incoming tangent is its mirror (-Handle[i]). A zero handle is a corner.
// The real outline is Outline() (curves flattened to a polyline).
type Object struct {
	Name   string
	Shape  []foil.Point
	Handle []foil.Point
	Pivot  foil.Point
	// Gaps, when len == len(Shape), marks per-edge cuts: Gaps[i] true means there
	// is no edge from Shape[i] to Shape[(i+1)%n]. Any gap makes the outline broken
	// (not a solid). Empty means a fully closed loop.
	Gaps []bool
	Keys []Key // sorted by T ascending; empty means a static identity pose
	// Control marks this object as directly driven by an external input (e.g. a
	// UI slider) instead of its keyframe track. PoseAt/PolygonAt still resolve
	// the authored keyframes as normal either way; interpreting Control -- and
	// deciding what pose to use instead -- is up to the host.
	Control bool
}

// Broken reports whether the outline has at least one edge cut, so it is not a
// fillable solid until reconnected.
func (o *Object) Broken() bool {
	if len(o.Gaps) != len(o.Shape) {
		return false
	}
	for _, g := range o.Gaps {
		if g {
			return true
		}
	}
	return false
}

// LooseEnd reports whether vertex v borders a gap (an edge cut on either side).
func (o *Object) LooseEnd(v int) bool {
	n := len(o.Shape)
	if len(o.Gaps) != n {
		return false
	}
	return o.Gaps[v] || o.Gaps[(v-1+n)%n]
}

// Segments returns the present edges (honoring Gaps) as individual flattened
// polylines, for drawing a possibly-broken outline in the editor.
func (o *Object) Segments() [][]foil.Point {
	n := len(o.Shape)
	if n < 2 {
		return nil
	}
	hasH := len(o.Handle) == n
	hasG := len(o.Gaps) == n
	var segs [][]foil.Point
	for i := range n {
		if hasG && o.Gaps[i] {
			continue
		}
		j := (i + 1) % n
		var h0, h1 foil.Point
		if hasH {
			h0, h1 = o.Handle[i], o.Handle[j]
		}
		segs = append(segs, flattenEdge(o.Shape[i], o.Shape[j], h0, h1))
	}
	return segs
}

// flattenEdge returns one edge as a polyline: two points for a straight edge, or
// a subdivided cubic when either endpoint has a tangent.
func flattenEdge(p0, p3, h0, h1 foil.Point) []foil.Point {
	if h0 == (foil.Point{}) && h1 == (foil.Point{}) {
		return []foil.Point{p0, p3}
	}
	c1 := foil.Point{X: p0.X + h0.X, Y: p0.Y + h0.Y}
	c2 := foil.Point{X: p3.X - h1.X, Y: p3.Y - h1.Y}
	out := make([]foil.Point, 0, bezSeg+1)
	for s := 0; s <= bezSeg; s++ {
		out = append(out, cubicAt(p0, c1, c2, p3, float64(s)/float64(bezSeg)))
	}
	return out
}

// bezSeg is the number of line segments emitted per curved edge when flattening.
const bezSeg = 16

// HasHandles reports whether the object has any non-zero Bezier tangent.
func (o *Object) HasHandles() bool {
	if len(o.Handle) != len(o.Shape) {
		return false
	}
	for _, h := range o.Handle {
		if h.X != 0 || h.Y != 0 {
			return true
		}
	}
	return false
}

// Outline returns the object's closed rest outline (the raw shape, or the closed
// cubic-Bezier path flattened). It ignores Gaps — broken objects are skipped
// before rasterization — so it stays valid for selection and bounds.
func (o *Object) Outline() []foil.Point {
	if !o.HasHandles() {
		return o.Shape
	}
	return flattenClosed(o.Shape, o.Handle)
}

// OutlineInto is Outline flattening into dst's storage, for callers that redo
// it every frame (the simulator's mask rebuild) and want no allocation after
// warm-up. Without handles it returns Shape directly, exactly like Outline.
func (o *Object) OutlineInto(dst []foil.Point) []foil.Point {
	if !o.HasHandles() {
		return o.Shape
	}
	return flattenClosedInto(dst[:0], o.Shape, o.Handle)
}

// AutoSmooth fills Handle with Catmull-Rom-equivalent tangents so the outline
// becomes smooth through every anchor (handle i = (P[i+1]-P[i-1])/6).
func (o *Object) AutoSmooth() {
	n := len(o.Shape)
	if n < 3 {
		return
	}
	o.Handle = make([]foil.Point, n)
	for i := range n {
		prev := o.Shape[(i-1+n)%n]
		next := o.Shape[(i+1)%n]
		o.Handle[i] = foil.Point{X: (next.X - prev.X) / 6, Y: (next.Y - prev.Y) / 6}
	}
}

// flattenClosed walks the closed path anchor->anchor, emitting a straight step
// for a corner edge (both tangents zero) or a subdivided cubic otherwise. Each
// edge contributes its start anchor and interior points; the next edge's start
// anchor closes it, so no point is duplicated.
func flattenClosed(anchor, handle []foil.Point) []foil.Point {
	n := len(anchor)
	if n < 2 {
		return anchor
	}
	return flattenClosedInto(make([]foil.Point, 0, n*bezSeg), anchor, handle)
}

// flattenClosedInto is flattenClosed appending into out, so a per-frame caller
// can reuse one buffer.
func flattenClosedInto(out, anchor, handle []foil.Point) []foil.Point {
	n := len(anchor)
	for i := range n {
		j := (i + 1) % n
		p0, p3 := anchor[i], anchor[j]
		h0, h1 := handle[i], handle[j]
		if h0 == (foil.Point{}) && h1 == (foil.Point{}) {
			out = append(out, p0)
			continue
		}
		c1 := foil.Point{X: p0.X + h0.X, Y: p0.Y + h0.Y}
		c2 := foil.Point{X: p3.X - h1.X, Y: p3.Y - h1.Y}
		for s := range bezSeg {
			out = append(out, cubicAt(p0, c1, c2, p3, float64(s)/float64(bezSeg)))
		}
	}
	return out
}

// OpenOutline flattens an OPEN path (no closing edge) through anchors with
// optional per-anchor tangents, for previewing an in-progress pen draft.
func OpenOutline(anchor, handle []foil.Point) []foil.Point {
	n := len(anchor)
	if n < 2 {
		return anchor
	}
	hasH := len(handle) == n
	out := make([]foil.Point, 0, n*bezSeg)
	for i := 0; i < n-1; i++ {
		p0, p3 := anchor[i], anchor[i+1]
		var h0, h1 foil.Point
		if hasH {
			h0, h1 = handle[i], handle[i+1]
		}
		if h0 == (foil.Point{}) && h1 == (foil.Point{}) {
			out = append(out, p0)
			continue
		}
		c1 := foil.Point{X: p0.X + h0.X, Y: p0.Y + h0.Y}
		c2 := foil.Point{X: p3.X - h1.X, Y: p3.Y - h1.Y}
		for s := range bezSeg {
			out = append(out, cubicAt(p0, c1, c2, p3, float64(s)/float64(bezSeg)))
		}
	}
	return append(out, anchor[n-1])
}

// cubicAt evaluates the cubic Bezier p0,c1,c2,p3 at t in [0,1].
func cubicAt(p0, c1, c2, p3 foil.Point, t float64) foil.Point {
	u := 1 - t
	a := u * u * u
	b := 3 * u * u * t
	c := 3 * u * t * t
	d := t * t * t
	return foil.Point{
		X: a*p0.X + b*c1.X + c*c2.X + d*p3.X,
		Y: a*p0.Y + b*c1.Y + c*c2.Y + d*p3.Y,
	}
}

// PoseAt returns the interpolated pose at time t. With no keys it is identity;
// outside the key range it clamps to the first or last key.
func (o *Object) PoseAt(t float64) Pose {
	if len(o.Keys) == 0 {
		return Identity()
	}
	if t <= o.Keys[0].T {
		return o.Keys[0].Pose
	}
	last := len(o.Keys) - 1
	if t >= o.Keys[last].T {
		return o.Keys[last].Pose
	}
	// First key strictly after t; interpolate from the one before it.
	i := sort.Search(len(o.Keys), func(i int) bool { return o.Keys[i].T > t })
	a, b := o.Keys[i-1], o.Keys[i]
	u := (t - a.T) / (b.T - a.T)
	return blend(a.Pose, b.Pose, u)
}

// PolygonAt returns the object's outline at time t with its pose applied.
func (o *Object) PolygonAt(t float64) []foil.Point {
	return Apply(o.Outline(), o.Pivot, o.PoseAt(t))
}

// PolygonAtInto is PolygonAt with caller-owned storage: outline holds the
// flattened shape and dst the posed result. Neither escapes; the returned
// slice aliases dst (or Shape when the pose is at rest and there is nothing to
// transform -- the same aliasing Outline itself does).
func (o *Object) PolygonAtInto(dst, outline []foil.Point, t float64) []foil.Point {
	return ApplyInto(dst[:0], o.OutlineInto(outline), o.Pivot, o.PoseAt(t))
}

// keyEps treats two key times within this distance as the same keyframe.
const keyEps = 1e-6

// SetKey inserts a keyframe at time t, or replaces the pose of an existing key
// at (approximately) that time. Keys stay sorted by time.
func (o *Object) SetKey(t float64, p Pose) {
	for i := range o.Keys {
		if math.Abs(o.Keys[i].T-t) <= keyEps {
			o.Keys[i].Pose = p
			return
		}
	}
	o.Keys = append(o.Keys, Key{T: t, Pose: p})
	sort.Slice(o.Keys, func(i, j int) bool { return o.Keys[i].T < o.Keys[j].T })
}

// DeleteKey removes the keyframe at (approximately) time t and reports whether
// one was found.
func (o *Object) DeleteKey(t float64) bool {
	for i := range o.Keys {
		if math.Abs(o.Keys[i].T-t) <= keyEps {
			o.Keys = append(o.Keys[:i], o.Keys[i+1:]...)
			return true
		}
	}
	return false
}

// Apply transforms pts by pose p: scale and rotate about pivot, then translate.
func Apply(pts []foil.Point, pivot foil.Point, p Pose) []foil.Point {
	return ApplyInto(make([]foil.Point, 0, len(pts)), pts, pivot, p)
}

// ApplyInto is Apply appending into dst, so a per-frame caller can reuse one
// buffer instead of allocating per transform.
func ApplyInto(dst, pts []foil.Point, pivot foil.Point, p Pose) []foil.Point {
	sin, cos := math.Sincos(p.Rot * math.Pi / 180)
	for _, q := range pts {
		x := (q.X - pivot.X) * p.Scale
		y := (q.Y - pivot.Y) * p.Scale
		rx := x*cos - y*sin
		ry := x*sin + y*cos
		dst = append(dst, foil.Point{X: pivot.X + rx + p.DX, Y: pivot.Y + ry + p.DY})
	}
	return dst
}

// Scene is a set of objects plus a loop length in seconds.
type Scene struct {
	Objects []*Object
	Loop    float64 // <= 0 means static (time is always 0)
}

// LoopTime wraps elapsed seconds into the [0, Loop) window.
func (s *Scene) LoopTime(elapsed float64) float64 {
	if s.Loop <= 0 {
		return 0
	}
	t := math.Mod(elapsed, s.Loop)
	if t < 0 {
		t += s.Loop
	}
	return t
}

// Mask rasterizes the union of all objects at time t into a solid mask laid out
// as y*nx+x.
func (s *Scene) Mask(t float64, nx, ny int) []bool {
	mask := make([]bool, nx*ny)
	for _, o := range s.Objects {
		if o.Broken() {
			continue // a cut outline is not a solid until it is closed
		}
		foil.RasterizeInto(mask, o.PolygonAt(t), nx, ny)
	}
	return mask
}
