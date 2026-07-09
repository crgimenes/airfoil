package main

import (
	"math"
	"os"
	"testing"

	"kutta/foil"
	"kutta/sceneio"
)

// TestSVGImportScene checks that normalized SVG outlines become one scene
// object each, placed like the interactive foil: chord cells wide starting at
// the leading-edge x, vertically centered on the grid.
func TestSVGImportScene(t *testing.T) {
	// Two normalized outlines the way ParseSVG hands them over: the group
	// spans x in [0,1] with y up and the vertical center at 0.
	outlines := [][]foil.Point{
		{{X: 0, Y: 0.1}, {X: 0.4, Y: 0.1}, {X: 0.4, Y: -0.1}, {X: 0, Y: -0.1}},
		{{X: 0.8, Y: 0.1}, {X: 1, Y: 0.1}, {X: 1, Y: -0.1}, {X: 0.8, Y: -0.1}},
	}
	sc := svgImportScene(outlines, "car")
	if len(sc.Objects) != 2 {
		t.Fatalf("got %d objects, want 2", len(sc.Objects))
	}
	if sc.Objects[0].Name != "car-1" || sc.Objects[1].Name != "car-2" {
		t.Errorf("object names: got %q, %q", sc.Objects[0].Name, sc.Objects[1].Name)
	}

	// The drawing spans exactly [leadX, leadX+chord] horizontally and stays
	// centered on the grid vertically.
	minX, maxX := math.Inf(1), math.Inf(-1)
	for _, o := range sc.Objects {
		for _, p := range o.Shape {
			minX = math.Min(minX, p.X)
			maxX = math.Max(maxX, p.X)
		}
	}
	leadX := leadXFrac * gridW
	chord := chordFrac * gridW
	if math.Abs(minX-leadX) > 1e-9 || math.Abs(maxX-(leadX+chord)) > 1e-9 {
		t.Errorf("x span: got [%g,%g], want [%g,%g]", minX, maxX, leadX, leadX+chord)
	}
	for _, o := range sc.Objects {
		for _, p := range o.Shape {
			if math.Abs(p.Y-gridH/2) > chord {
				t.Errorf("object %q strays far from the grid center: %+v", o.Name, p)
			}
		}
	}

	// Each pivot sits at its own object's centroid, so rotation in the editor
	// behaves like it does for drawn shapes.
	for _, o := range sc.Objects {
		c := centroid(o.Shape)
		if math.Abs(o.Pivot.X-c.X) > 1e-9 || math.Abs(o.Pivot.Y-c.Y) > 1e-9 {
			t.Errorf("object %q pivot: got %+v, want centroid %+v", o.Name, o.Pivot, c)
		}
	}
}

// TestCarExampleLoads guards examples/car.afoil (and the svg source it
// references) the way neko_test.go guards the neko demo.
func TestCarExampleLoads(t *testing.T) {
	b, err := os.ReadFile("examples/car.afoil")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sceneio.Load(string(b))
	if err != nil {
		t.Fatalf("car.afoil does not parse: %v", err)
	}
	if len(s.Objects) != 3 {
		t.Fatalf("got %d objects, want 3 (body and two wheels)", len(s.Objects))
	}
	mask := s.Mask(0, gridW, gridH)
	solid := 0
	for _, b := range mask {
		if b {
			solid++
		}
	}
	if solid == 0 {
		t.Error("car scene rasterized to nothing")
	}
}

// TestSVGImportSceneSingle checks a single outline keeps the plain name.
func TestSVGImportSceneSingle(t *testing.T) {
	outlines := [][]foil.Point{
		{{X: 0, Y: 0.25}, {X: 1, Y: 0.25}, {X: 1, Y: -0.25}, {X: 0, Y: -0.25}},
	}
	sc := svgImportScene(outlines, "car")
	if len(sc.Objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(sc.Objects))
	}
	if sc.Objects[0].Name != "car" {
		t.Errorf("single object name: got %q, want %q", sc.Objects[0].Name, "car")
	}
	if sc.Loop != 0 {
		t.Errorf("imported scene should be static, got loop %g", sc.Loop)
	}
}
