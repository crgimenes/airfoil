package sceneio

import (
	"fmt"
	"strconv"
	"strings"

	"kutta/scene"
)

// Save serializes a scene back to Filo text that Load reads back equivalently.
// Shapes are written as explicit point lists (a profile that was authored with
// (naca ...) is emitted as its resolved points), which is also what the editor
// produces, so the round trip is geometry-preserving.
func Save(s *scene.Scene) (string, error) {
	if s == nil {
		return "", fmt.Errorf("sceneio: nil scene")
	}
	var b strings.Builder
	b.WriteString("(scene\n")
	for _, o := range s.Objects {
		if len(o.Shape) < 3 {
			return "", fmt.Errorf("sceneio: object %q has no shape", o.Name)
		}
		fmt.Fprintf(&b, "  (object %q\n", o.Name)
		b.WriteString("    (shape")
		for _, p := range o.Shape {
			fmt.Fprintf(&b, " (point %s %s)", num(p.X), num(p.Y))
		}
		b.WriteString(")")
		if o.HasHandles() {
			b.WriteString("\n    (handles")
			for _, h := range o.Handle {
				fmt.Fprintf(&b, " (point %s %s)", num(h.X), num(h.Y))
			}
			b.WriteString(")")
		}
		if o.Broken() {
			b.WriteString("\n    (gaps")
			for i, gap := range o.Gaps {
				if gap {
					fmt.Fprintf(&b, " %d", i)
				}
			}
			b.WriteString(")")
		}
		if o.Control {
			b.WriteString("\n    (control)")
		}
		fmt.Fprintf(&b, "\n    (pivot %s %s)", num(o.Pivot.X), num(o.Pivot.Y))
		if len(o.Keys) > 0 {
			b.WriteString("\n    (keys")
			for _, k := range o.Keys {
				fmt.Fprintf(&b, "\n      (key %s (pose %s %s %s %s))",
					num(k.T), num(k.Pose.DX), num(k.Pose.DY), num(k.Pose.Rot), num(k.Pose.Scale))
			}
			b.WriteString(")")
		}
		b.WriteString(")\n")
	}
	fmt.Fprintf(&b, "  (loop %s))\n", num(s.Loop))
	return b.String(), nil
}

// num formats a float as the shortest string that parses back to the same value.
func num(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
