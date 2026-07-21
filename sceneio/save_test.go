package sceneio

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"kutta/foil"
	"kutta/scene"
)

func approxPt(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestRoundTrip loads the sample, saves it, loads it again, and checks the two
// scenes match — proving Save and Load are inverse for what the editor emits.
func TestRoundTrip(t *testing.T) {
	s1, err := Load(sample)
	if err != nil {
		t.Fatal(err)
	}
	text, err := Save(s1)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Load(text)
	if err != nil {
		t.Fatalf("reload saved scene: %v\n%s", err, text)
	}
	assertSceneEqual(t, s1, s2)
}

func assertSceneEqual(t *testing.T, a, b *scene.Scene) {
	t.Helper()
	if !approxPt(a.Loop, b.Loop) {
		t.Errorf("loop %g != %g", a.Loop, b.Loop)
	}
	if len(a.Objects) != len(b.Objects) {
		t.Fatalf("object count %d != %d", len(a.Objects), len(b.Objects))
	}
	for i := range a.Objects {
		oa, ob := a.Objects[i], b.Objects[i]
		if oa.Name != ob.Name {
			t.Errorf("object %d name %q != %q", i, oa.Name, ob.Name)
		}
		if len(oa.Shape) != len(ob.Shape) {
			t.Errorf("object %q shape len %d != %d", oa.Name, len(oa.Shape), len(ob.Shape))
		}
		if !approxPt(oa.Pivot.X, ob.Pivot.X) || !approxPt(oa.Pivot.Y, ob.Pivot.Y) {
			t.Errorf("object %q pivot %+v != %+v", oa.Name, oa.Pivot, ob.Pivot)
		}
		if len(oa.Handle) != len(ob.Handle) {
			t.Errorf("object %q handle len %d != %d", oa.Name, len(oa.Handle), len(ob.Handle))
		}
		if len(oa.Keys) != len(ob.Keys) {
			t.Fatalf("object %q key count %d != %d", oa.Name, len(oa.Keys), len(ob.Keys))
		}
		for k := range oa.Keys {
			ka, kb := oa.Keys[k], ob.Keys[k]
			if !approxPt(ka.T, kb.T) || ka.Pose != kb.Pose {
				t.Errorf("object %q key %d: %+v != %+v", oa.Name, k, ka, kb)
			}
		}
	}
}

// TestHandlesRoundTrip proves per-vertex Bezier tangents survive Save/Load.
func TestHandlesRoundTrip(t *testing.T) {
	s := &scene.Scene{
		Objects: []*scene.Object{{
			Name:   "blob",
			Shape:  []foil.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}},
			Handle: []foil.Point{{X: 1, Y: 2}, {}, {X: -3, Y: 0}, {}},
			Pivot:  foil.Point{X: 5, Y: 5},
		}},
	}
	text, err := Save(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(text)
	if err != nil {
		t.Fatalf("reload: %v\n%s", err, text)
	}
	h := got.Objects[0].Handle
	if len(h) != 4 || !approxPt(h[0].X, 1) || !approxPt(h[0].Y, 2) || !approxPt(h[2].X, -3) {
		t.Errorf("handles lost through round trip: %+v\n%s", h, text)
	}
}

// TestControlRoundTrip proves the Control flag survives Save/Load, and that it
// is off by default for an object that never set it.
func TestControlRoundTrip(t *testing.T) {
	s := &scene.Scene{
		Objects: []*scene.Object{
			{
				Name:    "flap",
				Shape:   []foil.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}},
				Pivot:   foil.Point{X: 5, Y: 5},
				Control: true,
			},
			{
				Name:  "wing",
				Shape: []foil.Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}},
				Pivot: foil.Point{X: 10, Y: 10},
			},
		},
	}
	text, err := Save(s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(text)
	if err != nil {
		t.Fatalf("reload: %v\n%s", err, text)
	}
	if !got.Objects[0].Control {
		t.Errorf("flap lost its Control flag through round trip:\n%s", text)
	}
	if got.Objects[1].Control {
		t.Errorf("wing should not be Control:\n%s", text)
	}
}

// TestLegacyCurvedMigrates proves an old (curved) file loads as smooth tangents.
func TestLegacyCurvedMigrates(t *testing.T) {
	src := `(scene (object "o"
		(shape (point 0 0) (point 10 0) (point 10 10) (point 0 10))
		(curved)) (loop 0))`
	got, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Objects[0].HasHandles() {
		t.Error("(curved) did not migrate to smooth handles")
	}
}

const seligSample = `TEST
1.0 0.0
0.5 0.06
0.0 0.0
0.5 -0.06
`

func TestDATSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.dat")
	err := os.WriteFile(path, []byte(seligSample), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// Forward slashes keep the path valid inside a Filo string on Windows,
	// where the native separator would read as escape sequences.
	src := `(scene (object "wing" (dat "` + filepath.ToSlash(path) + `" 100 10 50)) (loop 0))`
	s, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Objects) != 1 || len(s.Objects[0].Shape) != 4 {
		t.Fatalf("dat object not loaded: %+v", s.Objects)
	}
	// Scaled to chord 100 with LE at x=10, the trailing edge should sit near x=110.
	var maxX float64
	for _, p := range s.Objects[0].Shape {
		maxX = math.Max(maxX, p.X)
	}
	if maxX < 105 || maxX > 115 {
		t.Errorf("dat shape not placed/scaled as expected: maxX=%g", maxX)
	}
}

func TestDATMissingFile(t *testing.T) {
	src := `(scene (object "w" (dat "/no/such/file.dat" 100 0 0)) (loop 0))`
	_, err := Load(src)
	if err == nil {
		t.Error("expected an error for a missing .dat file")
	}
}
