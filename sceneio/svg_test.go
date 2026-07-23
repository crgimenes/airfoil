package sceneio

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two squares drawn together: x spans 0..25, so after group normalization the
// first square covers x 0..0.4 and the second 0.8..1.0 of the drawing width.
const twoBoxSVG = `<svg xmlns="http://www.w3.org/2000/svg">
  <path d="M0 0 H10 V5 H0 Z M20 0 h5 v5 h-5 z"/>
</svg>`

// writeSVG writes src to a temp file and returns the path with forward
// slashes, which Filo strings accept on every platform (backslashes would be
// taken as escapes).
func writeSVG(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.svg")
	err := os.WriteFile(path, []byte(src), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(path)
}

func TestSVGSource(t *testing.T) {
	path := writeSVG(t, twoBoxSVG)
	src := `(scene
	  (object "wing" (svg "` + path + `" 100 10 50))
	  (object "flap" (svg "` + path + `" 100 10 50 1))
	  (loop 0))`
	s, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Objects) != 2 {
		t.Fatalf("got %d objects, want 2", len(s.Objects))
	}
	// Subpath 0 spans x 0..0.4 of the width; scaled to 100 cells at leadX=10
	// that is grid x 10..50.
	var maxX float64
	for _, p := range s.Objects[0].Shape {
		maxX = math.Max(maxX, p.X)
	}
	if math.Abs(maxX-50) > 0.5 {
		t.Errorf("wing trailing edge: got maxX=%g, want ~50", maxX)
	}
	// Subpath 1 spans x 0.8..1.0: grid x 90..110 with the same placement, so
	// the two objects keep their drawn separation.
	var minX = math.Inf(1)
	for _, p := range s.Objects[1].Shape {
		minX = math.Min(minX, p.X)
	}
	if math.Abs(minX-90) > 0.5 {
		t.Errorf("flap leading edge: got minX=%g, want ~90", minX)
	}
}

func TestSVGMissingFile(t *testing.T) {
	src := `(scene (object "w" (svg "/no/such/file.svg" 100 0 0)) (loop 0))`
	_, err := Load(src)
	if err == nil {
		t.Error("expected an error for a missing .svg file")
	}
}

func TestSVGBadIndex(t *testing.T) {
	path := writeSVG(t, twoBoxSVG)
	src := `(scene (object "w" (svg "` + path + `" 100 0 0 5)) (loop 0))`
	_, err := Load(src)
	if err == nil {
		t.Fatal("expected an error for an out-of-range subpath index")
	}
	if !strings.Contains(err.Error(), "5") {
		t.Errorf("error should name the bad index: %v", err)
	}
}
