package foil

import (
	"math"
	"testing"
)

// approx reports whether two points match to a tolerance loose enough for
// curve flattening but tight enough to catch coordinate-mapping mistakes.
func approx(a, b Point) bool {
	return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9
}

// TestParseSVGRectPath checks the coordinate mapping on a plain rectangle:
// the drawing is normalized so its width spans x in [0,1], the y axis flips
// to point up, and the vertical center lands on y=0.
func TestParseSVGRectPath(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0 L10 0 L10 5 L0 5 Z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(outlines) != 1 {
		t.Fatalf("got %d outlines, want 1", len(outlines))
	}
	pts := outlines[0]
	if len(pts) != 4 {
		t.Fatalf("got %d points, want 4: %+v", len(pts), pts)
	}
	want := []Point{{0, 0.25}, {1, 0.25}, {1, -0.25}, {0, -0.25}}
	for i := range want {
		if !approx(pts[i], want[i]) {
			t.Errorf("point %d: got %+v, want %+v", i, pts[i], want[i])
		}
	}
}

// TestParseSVGImplicitLineto checks that extra coordinate pairs after M (and
// lowercase relative forms) are treated as linetos, per the SVG path grammar.
func TestParseSVGImplicitLineto(t *testing.T) {
	svg := `<svg><path d="M0 0 10 0 10 5 0 5 Z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(outlines) != 1 || len(outlines[0]) != 4 {
		t.Fatalf("implicit linetos: got %+v", outlines)
	}

	rel := `<svg><path d="m0 0 10 0 0 5 -10 0 z"/></svg>`
	relOut, err := ParseSVG([]byte(rel))
	if err != nil {
		t.Fatal(err)
	}
	if len(relOut) != 1 || len(relOut[0]) != 4 {
		t.Fatalf("relative implicit linetos: got %+v", relOut)
	}
	for i := range outlines[0] {
		if !approx(outlines[0][i], relOut[0][i]) {
			t.Errorf("point %d: absolute %+v != relative %+v", i, outlines[0][i], relOut[0][i])
		}
	}
}

// TestParseSVGMultiSubpath checks that subpaths become separate outlines but
// share one normalization, preserving their relative placement and scale.
func TestParseSVGMultiSubpath(t *testing.T) {
	svg := `<svg><path d="M0 0 H10 V5 H0 Z M20 0 h5 v5 h-5 z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(outlines) != 2 {
		t.Fatalf("got %d outlines, want 2", len(outlines))
	}
	// Group bbox is x 0..25, y 0..5: the second square starts at x=20/25=0.8,
	// and its SVG top edge (y=0) maps to +0.1 (half of 5/25) with y up.
	if !approx(outlines[1][0], Point{0.8, 0.1}) {
		t.Errorf("second subpath start: got %+v, want {0.8 0.1}", outlines[1][0])
	}
}

// TestParseSVGCubic checks that C/S curves are flattened into a polyline that
// passes near the curve rather than cutting straight between anchors.
func TestParseSVGCubic(t *testing.T) {
	// A symmetric arch from (0,10) to (10,10) bulging up to y=2.5 at mid-span
	// (cubic with both controls at y=0 peaks at 3/4 of the way to them).
	svg := `<svg><path d="M0 10 C0 0 10 0 10 10 Z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	pts := outlines[0]
	if len(pts) < 10 {
		t.Fatalf("cubic should flatten to many points, got %d", len(pts))
	}
	var maxY = math.Inf(-1)
	for _, p := range pts {
		maxY = math.Max(maxY, p.Y)
	}
	// Normalized: width 10, y spans 2.5..10 in SVG space. The arch top (SVG
	// y=2.5) maps to the highest +y after the flip: (6.25-2.5)/10 = 0.375.
	if math.Abs(maxY-0.375) > 0.01 {
		t.Errorf("arch peak: got maxY=%g, want ~0.375", maxY)
	}
}

// TestParseSVGQuadratic checks Q flattening via endpoints and the mid-curve.
func TestParseSVGQuadratic(t *testing.T) {
	svg := `<svg><path d="M0 10 Q5 0 10 10 Z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	pts := outlines[0]
	if len(pts) < 10 {
		t.Fatalf("quadratic should flatten to many points, got %d", len(pts))
	}
	var maxY = math.Inf(-1)
	for _, p := range pts {
		maxY = math.Max(maxY, p.Y)
	}
	// Quadratic peak at t=0.5 is y=5 in SVG space; y spans 5..10, center 7.5,
	// so the peak maps to (7.5-5)/10 = 0.25.
	if math.Abs(maxY-0.25) > 0.01 {
		t.Errorf("quadratic peak: got maxY=%g, want ~0.25", maxY)
	}
}

// TestParseSVGArc checks that A commands trace the arc, not the chord.
func TestParseSVGArc(t *testing.T) {
	// Two half-circle arcs of radius 5 forming a full circle.
	svg := `<svg><path d="M0 5 A5 5 0 0 1 10 5 A5 5 0 0 1 0 5 Z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	pts := outlines[0]
	if len(pts) < 12 {
		t.Fatalf("arcs should flatten to many points, got %d", len(pts))
	}
	// A unit-diameter circle centered on (0.5, 0): every point at radius 0.5.
	for _, p := range pts {
		r := math.Hypot(p.X-0.5, p.Y)
		if math.Abs(r-0.5) > 0.01 {
			t.Fatalf("arc point %+v is off the circle (r=%g)", p, r)
		}
	}
}

// TestParseSVGBasicShapes checks rect, circle, ellipse, polygon and polyline
// elements are converted to outlines.
func TestParseSVGBasicShapes(t *testing.T) {
	cases := []struct {
		name, svg string
	}{
		{"rect", `<svg><rect x="1" y="2" width="10" height="4"/></svg>`},
		{"circle", `<svg><circle cx="5" cy="5" r="5"/></svg>`},
		{"ellipse", `<svg><ellipse cx="5" cy="5" rx="5" ry="2"/></svg>`},
		{"polygon", `<svg><polygon points="0,0 10,0 10,5 0,5"/></svg>`},
		{"polyline", `<svg><polyline points="0,0 10,0 10,5"/></svg>`},
	}
	for _, c := range cases {
		outlines, err := ParseSVG([]byte(c.svg))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(outlines) != 1 || len(outlines[0]) < 3 {
			t.Errorf("%s: got %+v", c.name, outlines)
			continue
		}
		var minX, maxX = math.Inf(1), math.Inf(-1)
		for _, p := range outlines[0] {
			minX = math.Min(minX, p.X)
			maxX = math.Max(maxX, p.X)
		}
		if math.Abs(minX) > 0.01 || math.Abs(maxX-1) > 0.01 {
			t.Errorf("%s: x should span [0,1], got [%g,%g]", c.name, minX, maxX)
		}
	}
}

// TestParseSVGCompactNumbers checks the tokenizer on the terse output real
// tools emit: no spaces around negative signs, decimals without a leading
// zero, and exponents.
func TestParseSVGCompactNumbers(t *testing.T) {
	svg := `<svg><path d="M.5-.5L1e1-.5 10 5 .5 5Z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(outlines) != 1 || len(outlines[0]) != 4 {
		t.Fatalf("got %+v", outlines)
	}
}

// TestParseSVGSkipsDegenerate checks that subpaths with fewer than 3 distinct
// points are dropped, and consecutive duplicate points are merged.
func TestParseSVGSkipsDegenerate(t *testing.T) {
	svg := `<svg><path d="M0 0 L5 0 M0 0 L10 0 L10 0 L10 5 L0 5 Z"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(outlines) != 1 {
		t.Fatalf("degenerate subpath should be dropped: got %d outlines", len(outlines))
	}
	if len(outlines[0]) != 4 {
		t.Errorf("duplicate point should be merged: got %d points", len(outlines[0]))
	}
}

// TestParseSVGSkipsHidden checks that invisible elements are not imported:
// Inkscape files routinely carry hidden draft layers and paths, marked with
// display:none in a style or a display attribute, on the shape or a group.
func TestParseSVGSkipsHidden(t *testing.T) {
	svg := `<svg>
	  <path style="display:none;fill:#1e1ead" d="M0 0 H100 V50 H0 Z"/>
	  <path display="none" d="M0 0 H200 V50 H0 Z"/>
	  <g style="display:none"><rect x="0" y="0" width="300" height="50"/></g>
	  <g><rect x="0" y="0" width="10" height="5"/></g>
	</svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if len(outlines) != 1 {
		t.Fatalf("got %d outlines, want only the visible rect", len(outlines))
	}
	// If a hidden shape leaked in, it would dominate the group bbox and the
	// visible 10-wide rect would no longer span the normalized width.
	var maxX float64
	for _, p := range outlines[0] {
		maxX = math.Max(maxX, p.X)
	}
	if math.Abs(maxX-1) > 1e-9 {
		t.Errorf("visible rect should span the width: maxX=%g", maxX)
	}
}

// TestParseSVGErrors checks the failure modes: not XML, nothing drawable,
// only-degenerate geometry, a malformed path, and a zero-width drawing.
func TestParseSVGErrors(t *testing.T) {
	cases := map[string]string{
		"not xml":     `hello`,
		"no shapes":   `<svg><g/></svg>`,
		"degenerate":  `<svg><path d="M0 0 L1 0"/></svg>`,
		"malformed d": `<svg><path d="M0 0 Q"/></svg>`,
		"zero width":  `<svg><path d="M0 0 L0 5 L0 10 Z"/></svg>`,
		"bad number":  `<svg><path d="M0 0 Lfoo bar Z"/></svg>`,
	}
	for name, src := range cases {
		_, err := ParseSVG([]byte(src))
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// TestParseSVGRasterizes confirms an imported outline works end to end as a
// solver polygon, like TestParsedShapeRasterizes does for .dat files.
func TestParseSVGRasterizes(t *testing.T) {
	svg := `<svg><circle cx="5" cy="5" r="5"/></svg>`
	outlines, err := ParseSVG([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	placed := Place(outlines[0], 80, 10, 50, 0, 0)
	mask := Rasterize(placed, 120, 100)
	count := 0
	for _, b := range mask {
		if b {
			count++
		}
	}
	if count == 0 {
		t.Error("imported shape rasterized to nothing")
	}
}
