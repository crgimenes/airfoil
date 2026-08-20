// Package foil generates airfoil outlines and rasterizes them onto the solver
// grid. The headline source is the NACA 4-digit family, whose shape is a closed
// formula, so a whole "database" of profiles needs no stored data. Outlines are
// produced in chord-normalized coordinates (x in [0,1] along the chord, y up)
// and later placed, rotated by angle of attack, and sampled into a solid mask.
package foil

import (
	"fmt"
	"math"
	"slices"
)

// Point is a 2D coordinate in either chord-normalized or grid space.
type Point struct {
	X, Y float64
}

// NACA4 returns the closed outline of a NACA 4-digit airfoil for a code like
// "2412" or "0012". n is the number of samples per surface; cosine spacing packs
// more points near the leading edge where curvature is highest. The outline runs
// clockwise: upper surface from trailing edge to leading edge, then the lower
// surface back to the trailing edge, so it forms a single simple polygon.
func NACA4(code string, n int) ([]Point, error) {
	if len(code) != 4 {
		return nil, fmt.Errorf("foil: NACA code %q must be 4 digits", code)
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("foil: NACA code %q must be all digits", code)
		}
	}
	if n < 2 {
		return nil, fmt.Errorf("foil: need at least 2 samples per surface, got %d", n)
	}
	m := float64(code[0]-'0') / 100.0      // max camber, fraction of chord
	p := float64(code[1]-'0') / 10.0       // position of max camber
	t := float64(parse2(code[2:])) / 100.0 // max thickness, fraction of chord
	return buildOutline(n, t, func(x float64) (float64, float64) { return camber(m, p, x) }), nil
}

// NACA generates a NACA 4- or 5-digit airfoil, dispatching on the code length.
func NACA(code string, n int) ([]Point, error) {
	switch len(code) {
	case 4:
		return NACA4(code, n)
	case 5:
		return NACA5(code, n)
	}
	return nil, fmt.Errorf("foil: NACA code %q must be 4 or 5 digits", code)
}

// NACA5 generates the outline of a standard (non-reflex) NACA 5-digit airfoil,
// e.g. 23012. The digits are L P Q TT: L sets the design lift coefficient
// (0.15*L), the middle two digits set the position of max camber (their value
// divided by 200), Q=0 is the standard series (Q=1 reflex is unsupported), and
// TT is the thickness percent.
func NACA5(code string, n int) ([]Point, error) {
	if len(code) != 5 {
		return nil, fmt.Errorf("foil: NACA5 code %q must be 5 digits", code)
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return nil, fmt.Errorf("foil: NACA code %q must be all digits", code)
		}
	}
	if n < 2 {
		return nil, fmt.Errorf("foil: need at least 2 samples per surface, got %d", n)
	}
	if code[2] != '0' {
		return nil, fmt.Errorf("foil: reflex NACA 5-digit (%q) is not supported", code)
	}
	cl := 0.15 * float64(code[0]-'0') // design lift coefficient
	m, k1, ok := naca5Params(parse2(code[1:3]))
	if !ok {
		return nil, fmt.Errorf("foil: unsupported NACA 5-digit camber in %q", code)
	}
	k1 *= cl / 0.3 // the tabulated k1 is for a design Cl of 0.3
	t := float64(parse2(code[3:])) / 100.0
	return buildOutline(n, t, func(x float64) (float64, float64) { return camber5(m, k1, x) }), nil
}

// buildOutline samples the thickness and a mean line into a closed outline with
// cosine spacing, walking the upper surface TE->LE then the lower LE->TE.
func buildOutline(n int, t float64, camberFn func(x float64) (yc, dyc float64)) []Point {
	upper := make([]Point, 0, n+1)
	lower := make([]Point, 0, n+1)
	for j := 0; j <= n; j++ {
		beta := math.Pi * float64(j) / float64(n)
		x := 0.5 * (1 - math.Cos(beta)) // cosine spacing, 0..1
		yt := thickness(t, x)
		yc, dyc := camberFn(x)
		theta := math.Atan(dyc)
		sin, cos := math.Sincos(theta)
		upper = append(upper, Point{X: x - yt*sin, Y: yc + yt*cos})
		lower = append(lower, Point{X: x + yt*sin, Y: yc - yt*cos})
	}
	// Walk the upper surface from TE to LE, then the lower from LE to TE.
	outline := make([]Point, 0, 2*(n+1))
	for _, u := range slices.Backward(upper) {
		outline = append(outline, u)
	}
	for j := 1; j < len(lower); j++ {
		outline = append(outline, lower[j])
	}
	return outline
}

// naca5Params returns the mean-line parameters (m, k1) for a standard 5-digit
// camber, keyed by the middle two digits (the max-camber position code). The
// tabulated k1 is for a design Cl of 0.3.
func naca5Params(mid int) (m, k1 float64, ok bool) {
	switch mid {
	case 10:
		return 0.0580, 361.400, true
	case 20:
		return 0.1260, 51.640, true
	case 30:
		return 0.2025, 15.957, true
	case 40:
		return 0.2900, 6.643, true
	case 50:
		return 0.3910, 3.230, true
	}
	return 0, 0, false
}

// camber5 is the standard NACA 5-digit mean camber line and its slope.
func camber5(m, k1, x float64) (yc, dyc float64) {
	if x < m {
		yc = k1 / 6 * (x*x*x - 3*m*x*x + m*m*(3-m)*x)
		dyc = k1 / 6 * (3*x*x - 6*m*x + m*m*(3-m))
		return yc, dyc
	}
	yc = k1 / 6 * m * m * m * (1 - x)
	dyc = -k1 / 6 * m * m * m
	return yc, dyc
}

// thickness is the NACA 4-digit half-thickness distribution. The final
// coefficient (-0.1036) is the variant that closes the trailing edge, which
// keeps the rasterized body a single solid blob with no leak at the tail.
func thickness(t, x float64) float64 {
	return 5 * t * (0.2969*math.Sqrt(x) - 0.1260*x - 0.3516*x*x + 0.2843*x*x*x - 0.1036*x*x*x*x)
}

// camber returns the mean-camber-line height and its slope at x. A symmetric
// section (m or p zero) is flat, which also avoids a divide-by-zero.
func camber(m, p, x float64) (yc, dyc float64) {
	if m == 0 || p == 0 {
		return 0, 0
	}
	if x < p {
		yc = m / (p * p) * (2*p*x - x*x)
		dyc = 2 * m / (p * p) * (p - x)
		return yc, dyc
	}
	q := 1 - p
	yc = m / (q * q) * ((1 - 2*p) + 2*p*x - x*x)
	dyc = 2 * m / (q * q) * (p - x)
	return yc, dyc
}

func parse2(s string) int {
	return int(s[0]-'0')*10 + int(s[1]-'0')
}

// Place maps a chord-normalized outline into grid space. The chord is scaled to
// chord cells; the pivot — the point at chord fraction pivotFrac (0 = leading
// edge, 0.25 = quarter chord) — is pinned at (pivotX, pivotY); and the section
// is pitched by alpha radians about that pivot. Positive alpha is nose up
// (higher angle of attack) for a free stream flowing along +x with y up.
//
// Pivoting about the quarter chord (the aerodynamic center) keeps the rotation
// axis steady as the angle changes, which both looks natural and matches how an
// airfoil actually pitches.
func Place(outline []Point, chord, pivotX, pivotY, alpha, pivotFrac float64) []Point {
	sin, cos := math.Sincos(alpha)
	out := make([]Point, len(outline))
	for i, pt := range outline {
		x := (pt.X - pivotFrac) * chord
		y := pt.Y * chord
		// Clockwise rotation (by -alpha) lifts the leading edge relative to the
		// trailing edge, the convention pilots call a positive angle of attack.
		rx := x*cos + y*sin
		ry := -x*sin + y*cos
		out[i] = Point{X: pivotX + rx, Y: pivotY + ry}
	}
	return out
}

// Rasterize marks every grid cell whose center lies inside the polygon as solid,
// returning a mask laid out as y*nx+x for direct use by the solver. The scan is
// limited to the polygon's bounding box, so cost tracks the body, not the grid.
func Rasterize(poly []Point, nx, ny int) []bool {
	mask := make([]bool, nx*ny)
	RasterizeInto(mask, poly, nx, ny)
	return mask
}

// RasterizeInto ORs the polygon's interior into an existing mask (nx*ny), for
// building the union of several shapes. Cells already solid stay solid.
func RasterizeInto(mask []bool, poly []Point, nx, ny int) {
	if len(poly) < 3 {
		return
	}
	minX, minY := poly[0].X, poly[0].Y
	maxX, maxY := poly[0].X, poly[0].Y
	for _, pt := range poly[1:] {
		minX = math.Min(minX, pt.X)
		maxX = math.Max(maxX, pt.X)
		minY = math.Min(minY, pt.Y)
		maxY = math.Max(maxY, pt.Y)
	}
	x0 := clampInt(int(math.Floor(minX)), 0, nx-1)
	x1 := clampInt(int(math.Ceil(maxX)), 0, nx-1)
	y0 := clampInt(int(math.Floor(minY)), 0, ny-1)
	y1 := clampInt(int(math.Ceil(maxY)), 0, ny-1)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if pointInPolygon(float64(x)+0.5, float64(y)+0.5, poly) {
				mask[y*nx+x] = true
			}
		}
	}
}

// pointInPolygon is the standard even-odd ray-casting test against a closed
// polygon (the last vertex is implicitly joined to the first).
func pointInPolygon(px, py float64, poly []Point) bool {
	inside := false
	j := len(poly) - 1
	for i := range poly {
		yi := poly[i].Y
		yj := poly[j].Y
		if (yi > py) != (yj > py) {
			xCross := poly[i].X + (py-yi)/(yj-yi)*(poly[j].X-poly[i].X)
			if px < xCross {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
