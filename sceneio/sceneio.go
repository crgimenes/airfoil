// Package sceneio loads an animated scene from the Filo configuration language.
// Filo here is used as structured data (a tree of keyframe states), not as a
// program: builtins build a tagged value tree, which is decoded into a
// scene.Scene. Keeping this in its own package lets scene stay Filo-free.
//
// Grammar (Filo S-expressions; object names are strings):
//
//	(scene
//	  (object "wing" (naca "2412" 130 90 72))
//	  (object "flap"
//	    (shape (point 200 69) (point 240 69) (point 240 75) (point 200 75))
//	    (handles (point 0 0) (point 3 0) (point 0 0) (point -3 0)) ; optional
//	    (pivot 200 72)
//	    (keys
//	      (key 0 (pose 0 0 0 1))
//	      (key 2 (pose 0 0 -25 1))
//	      (key 4 (pose 0 0 0 1))))
//	  (loop 4))
//
// pose is (dx dy rot [scale]); coordinates are grid cells, rot is degrees CCW.
// handles gives each shape point a symmetric Bezier tangent (zero = corner); a
// legacy (curved) element is still accepted and migrated to smooth handles.
// gaps lists cut edge indices (a broken outline, not a solid); a legacy (open)
// element maps to a single gap at the wrap edge. control, when present, marks
// the object as host-driven (e.g. by a UI slider) instead of its keyframes.
package sceneio

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/crgimenes/filo"

	"kutta/foil"
	"kutta/scene"
)

// Ext is the file extension for airfoil scene files. The content is Filo
// syntax, but the extension is app-specific (plain .filo is any Filo program),
// for file association and the open/save dialog's type filter.
const Ext = ".afoil"

// Load parses a Filo scene description into a Scene.
func Load(src string) (*scene.Scene, error) {
	f := filo.New()
	defer f.Close()

	var result *scene.Scene
	var decodeErr error

	builtins := map[string]filo.Builtin{
		"point":   biPoint,
		"naca":    biNACA,
		"dat":     biDAT,
		"svg":     biSVG,
		"shape":   biTagged("shape"),
		"handles": biTagged("handles"),
		"gaps":    biTagged("gaps"),
		"open":    biTagged("open"),
		"curved":  biTagged("curved"),
		"control": biTagged("control"),
		"pivot":   biTagged("pivot"),
		"pose":    biTagged("pose"),
		"key":     biTagged("key"),
		"keys":    biTagged("keys"),
		"object":  biTagged("object"),
		"loop":    biTagged("loop"),
		"scene": func(_ context.Context, args []filo.Value) (filo.Value, error) {
			result, decodeErr = decodeScene(args)
			return filo.VNum(0), nil
		},
	}
	for name, fn := range builtins {
		err := f.RegisterBuiltin(name, fn)
		if err != nil {
			return nil, fmt.Errorf("sceneio: register %q: %w", name, err)
		}
	}

	err := f.DoString(src)
	if err != nil {
		return nil, fmt.Errorf("sceneio: %w", err)
	}
	if decodeErr != nil {
		return nil, decodeErr
	}
	if result == nil {
		return nil, fmt.Errorf("sceneio: no (scene ...) form found")
	}
	return result, nil
}

// biTagged returns a builtin that wraps its already-evaluated args in a tagged
// list, deferring all validation to the decoders.
func biTagged(tag string) filo.Builtin {
	return func(_ context.Context, args []filo.Value) (filo.Value, error) {
		return tagged(tag, args...), nil
	}
}

// biPoint builds a 2-number tuple from (point x y).
func biPoint(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 {
		return filo.Value{}, fmt.Errorf("point: want (point x y)")
	}
	x, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, fmt.Errorf("point x: %w", err)
	}
	y, err := args[1].AsNumber()
	if err != nil {
		return filo.Value{}, fmt.Errorf("point y: %w", err)
	}
	return filo.VTuple([]filo.Value{filo.VNum(x), filo.VNum(y)}), nil
}

// biNACA builds a shape from (naca "code" chord leadX leadY [samples]): a NACA
// 4-digit outline scaled to chord cells with its leading edge at (leadX, leadY).
func biNACA(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) < 4 {
		return filo.Value{}, fmt.Errorf("naca: want (naca \"code\" chord leadX leadY [n])")
	}
	if args[0].Kind != filo.KString {
		return filo.Value{}, fmt.Errorf("naca: code must be a string")
	}
	nums, err := numbers(args[1:])
	if err != nil {
		return filo.Value{}, fmt.Errorf("naca: %w", err)
	}
	chord, leadX, leadY := nums[0], nums[1], nums[2]
	samples := 80
	if len(nums) > 3 {
		samples = int(nums[3])
	}
	outline, err := foil.NACA(args[0].Str, samples)
	if err != nil {
		return filo.Value{}, err
	}
	placed := foil.Place(outline, chord, leadX, leadY, 0, 0)
	return shapeValue(placed), nil
}

// biDAT builds a shape from (dat "path" chord leadX leadY): an airfoil loaded
// from a .dat coordinate file, scaled to chord cells with its leading edge at
// (leadX, leadY). The path is resolved relative to the process working
// directory.
func biDAT(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 4 {
		return filo.Value{}, fmt.Errorf("dat: want (dat \"path\" chord leadX leadY)")
	}
	if args[0].Kind != filo.KString {
		return filo.Value{}, fmt.Errorf("dat: path must be a string")
	}
	nums, err := numbers(args[1:])
	if err != nil {
		return filo.Value{}, fmt.Errorf("dat: %w", err)
	}
	data, err := os.ReadFile(args[0].Str) // #nosec G304 -- path comes from the scene file the user opened
	if err != nil {
		return filo.Value{}, fmt.Errorf("dat: %w", err)
	}
	outline, err := foil.ParseDAT(data)
	if err != nil {
		return filo.Value{}, err
	}
	placed := foil.Place(outline, nums[0], nums[1], nums[2], 0, 0)
	return shapeValue(placed), nil
}

// biSVG builds a shape from (svg "path" chord leadX leadY [index]): an outline
// imported from an SVG file, scaled to chord cells with its left edge at
// (leadX, leadY). A drawing with several subpaths yields several outlines,
// normalized together; index (default 0) picks one, and giving each object the
// same placement arguments keeps their drawn relative positions.
func biSVG(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) < 4 || len(args) > 5 {
		return filo.Value{}, fmt.Errorf("svg: want (svg \"path\" chord leadX leadY [index])")
	}
	if args[0].Kind != filo.KString {
		return filo.Value{}, fmt.Errorf("svg: path must be a string")
	}
	nums, err := numbers(args[1:])
	if err != nil {
		return filo.Value{}, fmt.Errorf("svg: %w", err)
	}
	data, err := os.ReadFile(args[0].Str) // #nosec G304 -- path comes from the scene file the user opened
	if err != nil {
		return filo.Value{}, fmt.Errorf("svg: %w", err)
	}
	outlines, err := foil.ParseSVG(data)
	if err != nil {
		return filo.Value{}, err
	}
	idx := 0
	if len(nums) > 3 {
		idx = int(nums[3])
	}
	if idx < 0 || idx >= len(outlines) {
		return filo.Value{}, fmt.Errorf("svg: subpath index %d out of range (drawing has %d)", idx, len(outlines))
	}
	placed := foil.Place(outlines[idx], nums[0], nums[1], nums[2], 0, 0)
	return shapeValue(placed), nil
}

func tagged(tag string, items ...filo.Value) filo.Value {
	return filo.VList(append([]filo.Value{filo.VString(tag)}, items...))
}

func shapeValue(pts []foil.Point) filo.Value {
	items := make([]filo.Value, len(pts))
	for i, p := range pts {
		items[i] = filo.VTuple([]filo.Value{filo.VNum(p.X), filo.VNum(p.Y)})
	}
	return tagged("shape", items...)
}

// tagOf splits a tagged list into its tag and the remaining items.
func tagOf(v filo.Value) (string, []filo.Value, bool) {
	if v.Kind == filo.KList && len(v.List) > 0 && v.List[0].Kind == filo.KString {
		return v.List[0].Str, v.List[1:], true
	}
	return "", nil, false
}

func numbers(vs []filo.Value) ([]float64, error) {
	out := make([]float64, len(vs))
	for i, v := range vs {
		n, err := v.AsNumber()
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

func decodePoint(v filo.Value) (foil.Point, error) {
	if v.Kind != filo.KTuple || len(v.Tup) != 2 {
		return foil.Point{}, fmt.Errorf("expected a (point x y)")
	}
	x, _ := v.Tup[0].AsNumber()
	y, _ := v.Tup[1].AsNumber()
	return foil.Point{X: x, Y: y}, nil
}

func decodeShape(v filo.Value) ([]foil.Point, error) {
	tag, items, ok := tagOf(v)
	if !ok || tag != "shape" {
		return nil, fmt.Errorf("expected a shape or naca form")
	}
	pts := make([]foil.Point, len(items))
	for i, it := range items {
		p, err := decodePoint(it)
		if err != nil {
			return nil, err
		}
		pts[i] = p
	}
	if len(pts) < 3 {
		return nil, fmt.Errorf("shape needs at least 3 points")
	}
	return pts, nil
}

// decodePointList decodes any tagged list of (point x y) items, used for the
// handles element (which, unlike shape, has no minimum count).
func decodePointList(v filo.Value) ([]foil.Point, error) {
	_, items, ok := tagOf(v)
	if !ok {
		return nil, fmt.Errorf("expected a tagged point list")
	}
	pts := make([]foil.Point, len(items))
	for i, it := range items {
		p, err := decodePoint(it)
		if err != nil {
			return nil, err
		}
		pts[i] = p
	}
	return pts, nil
}

func decodePivot(v filo.Value) (foil.Point, error) {
	_, items, _ := tagOf(v)
	nums, err := numbers(items)
	if err != nil || len(nums) != 2 {
		return foil.Point{}, fmt.Errorf("pivot: want (pivot x y)")
	}
	return foil.Point{X: nums[0], Y: nums[1]}, nil
}

func decodePose(v filo.Value) (scene.Pose, error) {
	tag, items, ok := tagOf(v)
	if !ok || tag != "pose" {
		return scene.Pose{}, fmt.Errorf("expected a (pose ...) form")
	}
	nums, err := numbers(items)
	if err != nil || len(nums) < 3 {
		return scene.Pose{}, fmt.Errorf("pose: want (pose dx dy rot [scale])")
	}
	p := scene.Pose{DX: nums[0], DY: nums[1], Rot: nums[2], Scale: 1}
	if len(nums) > 3 {
		p.Scale = nums[3]
	}
	return p, nil
}

func decodeKeys(v filo.Value) ([]scene.Key, error) {
	_, items, _ := tagOf(v)
	keys := make([]scene.Key, 0, len(items))
	for _, it := range items {
		tag, kitems, ok := tagOf(it)
		if !ok || tag != "key" || len(kitems) != 2 {
			return nil, fmt.Errorf("keys: want (key t (pose ...))")
		}
		t, err := kitems[0].AsNumber()
		if err != nil {
			return nil, fmt.Errorf("key time: %w", err)
		}
		pose, err := decodePose(kitems[1])
		if err != nil {
			return nil, err
		}
		keys = append(keys, scene.Key{T: t, Pose: pose})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].T < keys[j].T })
	return keys, nil
}

func decodeObject(items []filo.Value) (*scene.Object, error) {
	if len(items) < 2 || items[0].Kind != filo.KString {
		return nil, fmt.Errorf("object: want (object \"name\" ...)")
	}
	obj := &scene.Object{Name: items[0].Str}
	havePivot := false
	autoSmooth := false
	legacyOpen := false
	var gapIdx []int
	for _, it := range items[1:] {
		tag, _, ok := tagOf(it)
		if !ok {
			return nil, fmt.Errorf("object %q: unexpected element", obj.Name)
		}
		switch tag {
		case "shape":
			pts, err := decodeShape(it)
			if err != nil {
				return nil, fmt.Errorf("object %q: %w", obj.Name, err)
			}
			obj.Shape = pts
		case "pivot":
			pv, err := decodePivot(it)
			if err != nil {
				return nil, fmt.Errorf("object %q: %w", obj.Name, err)
			}
			obj.Pivot = pv
			havePivot = true
		case "keys":
			keys, err := decodeKeys(it)
			if err != nil {
				return nil, fmt.Errorf("object %q: %w", obj.Name, err)
			}
			obj.Keys = keys
		case "handles":
			pts, err := decodePointList(it)
			if err != nil {
				return nil, fmt.Errorf("object %q handles: %w", obj.Name, err)
			}
			obj.Handle = pts
		case "gaps":
			_, gi, _ := tagOf(it)
			nums, err := numbers(gi)
			if err != nil {
				return nil, fmt.Errorf("object %q gaps: %w", obj.Name, err)
			}
			for _, v := range nums {
				gapIdx = append(gapIdx, int(v))
			}
		case "open":
			legacyOpen = true // legacy single break at the wrap edge
		case "curved":
			autoSmooth = true // legacy flag: derive smooth tangents below
		case "control":
			obj.Control = true
		default:
			return nil, fmt.Errorf("object %q: unknown element %q", obj.Name, tag)
		}
	}
	if len(obj.Shape) < 3 {
		return nil, fmt.Errorf("object %q: missing shape", obj.Name)
	}
	if len(obj.Handle) != 0 && len(obj.Handle) != len(obj.Shape) {
		return nil, fmt.Errorf("object %q: %d handles for %d points", obj.Name, len(obj.Handle), len(obj.Shape))
	}
	if autoSmooth && len(obj.Handle) == 0 {
		obj.AutoSmooth()
	}
	if len(gapIdx) > 0 || legacyOpen {
		obj.Gaps = make([]bool, len(obj.Shape))
		for _, gi := range gapIdx {
			if gi >= 0 && gi < len(obj.Gaps) {
				obj.Gaps[gi] = true
			}
		}
		if legacyOpen {
			obj.Gaps[len(obj.Gaps)-1] = true // the missing wrap edge
		}
	}
	if !havePivot {
		obj.Pivot = centroid(obj.Shape)
	}
	return obj, nil
}

func decodeScene(args []filo.Value) (*scene.Scene, error) {
	s := &scene.Scene{}
	for _, a := range args {
		tag, items, ok := tagOf(a)
		if !ok {
			return nil, fmt.Errorf("scene: unexpected element")
		}
		switch tag {
		case "object":
			obj, err := decodeObject(items)
			if err != nil {
				return nil, err
			}
			s.Objects = append(s.Objects, obj)
		case "loop":
			nums, err := numbers(items)
			if err != nil || len(nums) != 1 {
				return nil, fmt.Errorf("loop: want (loop seconds)")
			}
			s.Loop = nums[0]
		default:
			return nil, fmt.Errorf("scene: unknown element %q", tag)
		}
	}
	if len(s.Objects) == 0 {
		return nil, fmt.Errorf("scene: no objects")
	}
	return s, nil
}

// centroid is the average vertex, the default pivot when none is given.
func centroid(pts []foil.Point) foil.Point {
	var cx, cy float64
	for _, p := range pts {
		cx += p.X
		cy += p.Y
	}
	n := float64(len(pts))
	return foil.Point{X: cx / n, Y: cy / n}
}
