package foil

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// ParseSVG reads an SVG document into closed outlines, one per subpath or
// basic shape, ready for Place and Rasterize. Curves (cubic, quadratic, arc)
// are flattened to polylines, so what comes out is plain polygons.
//
// The whole drawing is normalized as a group into the same convention .dat
// files use: x spans [0,1] across the drawing's width, y points up (SVG's y
// points down), and the vertical center sits at y=0. Normalizing as a group
// preserves the relative placement of multiple shapes, so a wing plus flap
// drawn together imports with the gap intact.
//
// Supported: path (M L H V C S Q T A Z and their relative forms), rect,
// circle, ellipse, polygon and polyline elements. Not interpreted: transform
// attributes, fill rules (holes become solid) and strokes (only the outline
// geometry is used) — flatten transforms in the editor before exporting.
func ParseSVG(data []byte) ([][]Point, error) {
	raw, err := collectShapes(data)
	if err != nil {
		return nil, err
	}
	outlines := make([][]Point, 0, len(raw))
	for _, sp := range raw {
		sp = dedup(sp)
		if len(sp) >= 3 {
			outlines = append(outlines, sp)
		}
	}
	if len(outlines) == 0 {
		return nil, fmt.Errorf("foil: svg has no drawable outlines")
	}
	return normalize(outlines)
}

// collectShapes walks the XML and converts every drawable element into one or
// more subpaths in raw SVG coordinates (y down).
func collectShapes(data []byte) ([][]Point, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var out [][]Point
	// hidden tracks display:none through nesting: each open element records
	// whether it or any ancestor is invisible, so hidden draft layers common
	// in Inkscape files do not leak into the import.
	var hidden []bool
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("foil: svg: %w", err)
		}
		_, isEnd := tok.(xml.EndElement)
		if isEnd {
			if len(hidden) > 0 {
				hidden = hidden[:len(hidden)-1]
			}
			continue
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		invisible := (len(hidden) > 0 && hidden[len(hidden)-1]) || elementHidden(el)
		hidden = append(hidden, invisible)
		if invisible {
			continue
		}
		var subs [][]Point
		switch el.Name.Local {
		case "path":
			subs, err = parsePath(attr(el, "d"))
		case "rect":
			subs, err = parseRect(el)
		case "circle":
			subs, err = parseEllipseShape(el, "r", "r")
		case "ellipse":
			subs, err = parseEllipseShape(el, "rx", "ry")
		case "polygon", "polyline":
			subs, err = parsePoints(attr(el, "points"))
		}
		if err != nil {
			return nil, fmt.Errorf("foil: svg <%s>: %w", el.Name.Local, err)
		}
		out = append(out, subs...)
	}
}

// elementHidden reports whether the element is invisible via a display="none"
// attribute or a display:none declaration in its style attribute.
func elementHidden(el xml.StartElement) bool {
	if strings.TrimSpace(attr(el, "display")) == "none" {
		return true
	}
	for decl := range strings.SplitSeq(attr(el, "style"), ";") {
		prop, val, ok := strings.Cut(decl, ":")
		if ok && strings.TrimSpace(prop) == "display" && strings.TrimSpace(val) == "none" {
			return true
		}
	}
	return false
}

func attr(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func attrNum(el xml.StartElement, name string) (float64, error) {
	s := attr(el, name)
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("attribute %s=%q: %w", name, s, err)
	}
	return v, nil
}

func parseRect(el xml.StartElement) ([][]Point, error) {
	var vals [4]float64
	for i, name := range []string{"x", "y", "width", "height"} {
		v, err := attrNum(el, name)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	x, y, w, h := vals[0], vals[1], vals[2], vals[3]
	if w <= 0 || h <= 0 {
		return nil, nil
	}
	return [][]Point{{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}}, nil
}

// ellipseSegments is the number of samples for a full circle or ellipse.
const ellipseSegments = 64

func parseEllipseShape(el xml.StartElement, rxName, ryName string) ([][]Point, error) {
	cx, err := attrNum(el, "cx")
	if err != nil {
		return nil, err
	}
	cy, err := attrNum(el, "cy")
	if err != nil {
		return nil, err
	}
	rx, err := attrNum(el, rxName)
	if err != nil {
		return nil, err
	}
	ry, err := attrNum(el, ryName)
	if err != nil {
		return nil, err
	}
	if rx <= 0 || ry <= 0 {
		return nil, nil
	}
	pts := make([]Point, ellipseSegments)
	for i := range pts {
		sin, cos := math.Sincos(2 * math.Pi * float64(i) / ellipseSegments)
		pts[i] = Point{X: cx + rx*cos, Y: cy + ry*sin}
	}
	return [][]Point{pts}, nil
}

func parsePoints(s string) ([][]Point, error) {
	sc := &pathScanner{s: s}
	var pts []Point
	for sc.hasNumber() {
		p, err := sc.pair()
		if err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	if len(pts) == 0 {
		return nil, nil
	}
	return [][]Point{pts}, nil
}

// curveSegments is the number of line segments emitted when flattening one
// cubic or quadratic curve command, matching the editor's flattening density.
const curveSegments = 16

// pathBuilder accumulates subpaths while walking path commands, tracking the
// state the SVG path grammar needs: the current point, the subpath start (for
// Z), and the previous control point (for the S/T reflected shorthands).
type pathBuilder struct {
	subs     [][]Point
	sp       []Point
	cur      Point
	start    Point
	prevCtrl Point
	lastCmd  byte
}

// flush ends the current subpath, keeping it if it has any geometry.
func (b *pathBuilder) flush() {
	if len(b.sp) > 0 {
		b.subs = append(b.subs, b.sp)
		b.sp = nil
	}
}

// moveTo starts a new subpath at p.
func (b *pathBuilder) moveTo(p Point) {
	b.flush()
	b.cur = p
	b.start = p
	b.sp = []Point{p}
}

// add appends a point reached by a draw command, opening the subpath at the
// current point if a draw follows a Z with no intervening M.
func (b *pathBuilder) add(p Point) {
	if len(b.sp) == 0 {
		b.sp = append(b.sp, b.cur)
	}
	b.sp = append(b.sp, p)
	b.cur = p
}

// cubic flattens the cubic Bezier from the current point via c1, c2 to p3.
func (b *pathBuilder) cubic(c1, c2, p3 Point) {
	p0 := b.cur
	for s := 1; s <= curveSegments; s++ {
		t := float64(s) / curveSegments
		u := 1 - t
		b.add(Point{
			X: u*u*u*p0.X + 3*u*u*t*c1.X + 3*u*t*t*c2.X + t*t*t*p3.X,
			Y: u*u*u*p0.Y + 3*u*u*t*c1.Y + 3*u*t*t*c2.Y + t*t*t*p3.Y,
		})
	}
	b.prevCtrl = c2
}

// quad flattens the quadratic Bezier from the current point via c to p2.
func (b *pathBuilder) quad(c, p2 Point) {
	p0 := b.cur
	for s := 1; s <= curveSegments; s++ {
		t := float64(s) / curveSegments
		u := 1 - t
		b.add(Point{
			X: u*u*p0.X + 2*u*t*c.X + t*t*p2.X,
			Y: u*u*p0.Y + 2*u*t*c.Y + t*t*p2.Y,
		})
	}
	b.prevCtrl = c
}

// reflect returns the previous control point mirrored about the current
// point, or the current point itself when the previous command was not of
// the given family — the SVG rule for the S and T shorthands.
func (b *pathBuilder) reflect(family string) Point {
	if strings.IndexByte(family, b.lastCmd) < 0 {
		return b.cur
	}
	return Point{X: 2*b.cur.X - b.prevCtrl.X, Y: 2*b.cur.Y - b.prevCtrl.Y}
}

// parsePath converts one path d attribute into subpaths of raw SVG points.
func parsePath(d string) ([][]Point, error) {
	sc := &pathScanner{s: d}
	b := &pathBuilder{}
	for {
		cmd, ok := sc.command()
		if !ok {
			break
		}
		rel := cmd >= 'a' // lowercase commands use coordinates relative to cur
		base := func() Point {
			if rel {
				return b.cur
			}
			return Point{}
		}
		up := cmd &^ 0x20 // uppercase for dispatch
		// Each command reads one argument group, then repeats while more
		// numbers follow (the grammar's implicit command repetition).
		for first := true; first || sc.hasNumber(); first = false {
			switch up {
			case 'M':
				p, err := sc.pair()
				if err != nil {
					return nil, err
				}
				p = offset(p, base())
				if first {
					b.moveTo(p)
				} else {
					b.add(p) // extra pairs after moveto are linetos
				}
			case 'L':
				p, err := sc.pair()
				if err != nil {
					return nil, err
				}
				b.add(offset(p, base()))
			case 'H':
				v, err := sc.number()
				if err != nil {
					return nil, err
				}
				if rel {
					v += b.cur.X
				}
				b.add(Point{X: v, Y: b.cur.Y})
			case 'V':
				v, err := sc.number()
				if err != nil {
					return nil, err
				}
				if rel {
					v += b.cur.Y
				}
				b.add(Point{X: b.cur.X, Y: v})
			case 'C':
				c1, err := sc.pair()
				if err != nil {
					return nil, err
				}
				c2, err := sc.pair()
				if err != nil {
					return nil, err
				}
				p, err := sc.pair()
				if err != nil {
					return nil, err
				}
				b.cubic(offset(c1, base()), offset(c2, base()), offset(p, base()))
			case 'S':
				c1 := b.reflect("CcSs")
				c2, err := sc.pair()
				if err != nil {
					return nil, err
				}
				p, err := sc.pair()
				if err != nil {
					return nil, err
				}
				b.cubic(c1, offset(c2, base()), offset(p, base()))
			case 'Q':
				c, err := sc.pair()
				if err != nil {
					return nil, err
				}
				p, err := sc.pair()
				if err != nil {
					return nil, err
				}
				b.quad(offset(c, base()), offset(p, base()))
			case 'T':
				c := b.reflect("QqTt")
				p, err := sc.pair()
				if err != nil {
					return nil, err
				}
				b.quad(c, offset(p, base()))
			case 'A':
				err := parseArc(sc, b, rel)
				if err != nil {
					return nil, err
				}
			case 'Z':
				b.flush()
				b.cur = b.start
			default:
				return nil, fmt.Errorf("unknown path command %q", string(cmd))
			}
			b.lastCmd = cmd
			if up == 'Z' {
				break // Z takes no arguments, so it never repeats
			}
		}
	}
	b.flush()
	return b.subs, nil
}

func offset(p, by Point) Point { return Point{X: p.X + by.X, Y: p.Y + by.Y} }

// parseArc reads one elliptical-arc argument group and flattens it using the
// endpoint-to-center conversion from the SVG spec (appendix F.6.5).
func parseArc(sc *pathScanner, b *pathBuilder, rel bool) error {
	rx, err := sc.number()
	if err != nil {
		return err
	}
	ry, err := sc.number()
	if err != nil {
		return err
	}
	rotDeg, err := sc.number()
	if err != nil {
		return err
	}
	large, err := sc.flag()
	if err != nil {
		return err
	}
	sweep, err := sc.flag()
	if err != nil {
		return err
	}
	end, err := sc.pair()
	if err != nil {
		return err
	}
	if rel {
		end = offset(end, b.cur)
	}
	p0 := b.cur
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 || p0 == end {
		b.add(end) // per spec, a degenerate arc is a straight line
		return nil
	}
	sinR, cosR := math.Sincos(rotDeg * math.Pi / 180)

	// Transform to the ellipse frame and find the center candidate.
	dx, dy := (p0.X-end.X)/2, (p0.Y-end.Y)/2
	x1 := cosR*dx + sinR*dy
	y1 := -sinR*dx + cosR*dy
	// Scale radii up if they cannot span the endpoints.
	lambda := x1*x1/(rx*rx) + y1*y1/(ry*ry)
	if lambda > 1 {
		s := math.Sqrt(lambda)
		rx *= s
		ry *= s
	}
	num := rx*rx*ry*ry - rx*rx*y1*y1 - ry*ry*x1*x1
	den := rx*rx*y1*y1 + ry*ry*x1*x1
	co := math.Sqrt(math.Max(0, num/den))
	if large == sweep {
		co = -co
	}
	cx1 := co * rx * y1 / ry
	cy1 := -co * ry * x1 / rx
	cx := cosR*cx1 - sinR*cy1 + (p0.X+end.X)/2
	cy := sinR*cx1 + cosR*cy1 + (p0.Y+end.Y)/2

	ang := func(ux, uy, vx, vy float64) float64 {
		return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	}
	ux, uy := (x1-cx1)/rx, (y1-cy1)/ry
	theta := ang(1, 0, ux, uy)
	delta := ang(ux, uy, (-x1-cx1)/rx, (-y1-cy1)/ry)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	}
	if sweep && delta < 0 {
		delta += 2 * math.Pi
	}

	n := max(int(math.Ceil(math.Abs(delta)/(2*math.Pi)*ellipseSegments)), 2)
	for s := 1; s <= n; s++ {
		sinT, cosT := math.Sincos(theta + delta*float64(s)/float64(n))
		b.add(Point{
			X: cx + rx*cosT*cosR - ry*sinT*sinR,
			Y: cy + rx*cosT*sinR + ry*sinT*cosR,
		})
	}
	b.add(end) // land exactly on the endpoint despite flattening error
	return nil
}

// dedup drops consecutive duplicate points and an explicit closing point that
// repeats the first, since the rasterizer closes the loop implicitly.
func dedup(pts []Point) []Point {
	const eps = 1e-9
	same := func(a, b Point) bool {
		return math.Abs(a.X-b.X) < eps && math.Abs(a.Y-b.Y) < eps
	}
	out := pts[:0]
	for _, p := range pts {
		if len(out) == 0 || !same(out[len(out)-1], p) {
			out = append(out, p)
		}
	}
	if len(out) > 1 && same(out[0], out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}

// normalize maps all outlines together from SVG coordinates into the .dat
// convention: x in [0,1] across the group's width, y up, vertical center at 0.
func normalize(outlines [][]Point) ([][]Point, error) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, sp := range outlines {
		for _, p := range sp {
			minX = math.Min(minX, p.X)
			maxX = math.Max(maxX, p.X)
			minY = math.Min(minY, p.Y)
			maxY = math.Max(maxY, p.Y)
		}
	}
	w := maxX - minX
	if w <= 0 || math.IsInf(w, 0) || math.IsNaN(w) {
		return nil, fmt.Errorf("foil: svg drawing has no width to scale by")
	}
	midY := (minY + maxY) / 2
	for _, sp := range outlines {
		for i, p := range sp {
			sp[i] = Point{X: (p.X - minX) / w, Y: (midY - p.Y) / w}
		}
	}
	return outlines, nil
}

// pathScanner tokenizes SVG path data: command letters, numbers in the CSS
// syntax (sign, decimals without a leading zero, exponents), arc flags, and
// comma-or-whitespace separators between all of them.
type pathScanner struct {
	s string
	i int
}

func (sc *pathScanner) skipSep() {
	for sc.i < len(sc.s) {
		c := sc.s[sc.i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != ',' {
			return
		}
		sc.i++
	}
}

// command returns the next command letter, or false at the end of the data.
func (sc *pathScanner) command() (byte, bool) {
	sc.skipSep()
	if sc.i >= len(sc.s) {
		return 0, false
	}
	c := sc.s[sc.i]
	sc.i++
	return c, true
}

// hasNumber reports whether the next token starts a number, which is how the
// grammar signals an implicit repetition of the current command.
func (sc *pathScanner) hasNumber() bool {
	sc.skipSep()
	if sc.i >= len(sc.s) {
		return false
	}
	c := sc.s[sc.i]
	return c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9')
}

func (sc *pathScanner) number() (float64, error) {
	sc.skipSep()
	start := sc.i
	if sc.i < len(sc.s) && (sc.s[sc.i] == '+' || sc.s[sc.i] == '-') {
		sc.i++
	}
	digits := false
	for sc.i < len(sc.s) && sc.s[sc.i] >= '0' && sc.s[sc.i] <= '9' {
		sc.i++
		digits = true
	}
	if sc.i < len(sc.s) && sc.s[sc.i] == '.' {
		sc.i++
		for sc.i < len(sc.s) && sc.s[sc.i] >= '0' && sc.s[sc.i] <= '9' {
			sc.i++
			digits = true
		}
	}
	if !digits {
		return 0, fmt.Errorf("expected a number at %q", sc.s[start:min(start+8, len(sc.s))])
	}
	if sc.i < len(sc.s) && (sc.s[sc.i] == 'e' || sc.s[sc.i] == 'E') {
		j := sc.i + 1
		if j < len(sc.s) && (sc.s[j] == '+' || sc.s[j] == '-') {
			j++
		}
		if j < len(sc.s) && sc.s[j] >= '0' && sc.s[j] <= '9' {
			for j < len(sc.s) && sc.s[j] >= '0' && sc.s[j] <= '9' {
				j++
			}
			sc.i = j
		}
	}
	v, err := strconv.ParseFloat(sc.s[start:sc.i], 64)
	if err != nil {
		return 0, fmt.Errorf("bad number %q", sc.s[start:sc.i])
	}
	return v, nil
}

func (sc *pathScanner) pair() (Point, error) {
	x, err := sc.number()
	if err != nil {
		return Point{}, err
	}
	y, err := sc.number()
	if err != nil {
		return Point{}, err
	}
	return Point{X: x, Y: y}, nil
}

// flag reads an arc flag, which is a bare 0 or 1 that may be run together
// with the next number ("a1 1 0 011 0"), so it must consume exactly one char.
func (sc *pathScanner) flag() (bool, error) {
	sc.skipSep()
	if sc.i >= len(sc.s) || (sc.s[sc.i] != '0' && sc.s[sc.i] != '1') {
		return false, fmt.Errorf("expected an arc flag (0 or 1)")
	}
	sc.i++
	return sc.s[sc.i-1] == '1', nil
}
