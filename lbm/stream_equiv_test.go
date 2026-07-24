package lbm

import "testing"

// streamReference is the original general-form streaming loop, kept as the
// reference the specialized interior/border split must match bit for bit.
func (s *Solver) streamReference() {
	nx, ny := s.NX, s.NY
	for y := range ny {
		for x := range nx {
			c := y*nx + x
			if s.solid[c] {
				for i := range 9 {
					s.ftmp[i][c] = s.f[i][c]
				}
				continue
			}
			for i := range 9 {
				sx := x - exi[i]
				sy := y - eyi[i]
				if sx < 0 || sx >= nx || sy < 0 || sy >= ny {
					s.ftmp[i][c] = s.f[i][c]
					continue
				}
				src := sy*nx + sx
				if s.solid[src] {
					s.ftmp[i][c] = s.f[opp[i]][c]
					continue
				}
				s.ftmp[i][c] = s.f[i][src]
			}
		}
	}
}

// TestStreamMatchesReference pins the specialized stream to the general loop,
// bit for bit, on both bench bodies and on a body touching the domain border.
func TestStreamMatchesReference(t *testing.T) {
	shapes := []func() *Solver{
		func() *Solver { return benchSolver(false) },
		func() *Solver { return benchSolver(true) },
		func() *Solver {
			s := New(90, 60, 0.6, 0.1)
			mask := make([]bool, 90*60)
			// A body hugging the walls exercises the border interplay.
			for y := range 60 {
				for x := 40; x < 46; x++ {
					mask[y*90+x] = true
				}
			}
			s.SetSolid(mask)
			return s
		},
	}
	for si, mk := range shapes {
		s := mk()
		for range 3 {
			s.Step()
		}
		s.collide(true)
		s.applyBoundaries()
		s.computeForce()

		ref := make([][]float32, 9)
		s.streamReference()
		for i := range 9 {
			ref[i] = append([]float32(nil), s.ftmp[i]...)
		}
		s.stream()
		for i := range 9 {
			for c := range ref[i] {
				if s.ftmp[i][c] != ref[i][c] {
					t.Fatalf("shape %d: direction %d cell %d: specialized %v, reference %v", si, i, c, s.ftmp[i][c], ref[i][c])
				}
			}
		}
	}
}
