//go:build !js

package lbm

import "testing"

// legacyStep is the collide / apply-boundaries / stream sequence the fused
// kernel replaces, kept here as the reference it has to match bit for bit.
func (s *Solver) legacyStep(store bool) {
	s.collide(store)
	s.applyBoundaries()
	s.stream()
}

// wallHugger is a body that touches the top and bottom edges, so the fused
// kernel's boundary substitution and its bounce-back meet at the same cells.
func wallHugger() *Solver {
	s := New(90, 60, 0.6, 0.1)
	mask := make([]bool, 90*60)
	for y := range 60 {
		for x := 40; x < 46; x++ {
			mask[y*90+x] = true
		}
	}
	s.SetSolid(mask)
	return s
}

// outflowHugger puts the body a few cells from the right edge, so the flow is
// still recovering where the outflow column copies its neighbour. Without it
// the far wake is uniform, the copied values come out bit-identical, and the
// test cannot see a wrong outflow at all.
func outflowHugger() *Solver {
	s := New(90, 60, 0.6, 0.1)
	mask := make([]bool, 90*60)
	for y := 20; y < 40; y++ {
		for x := 75; x < 85; x++ {
			mask[y*90+x] = true
		}
	}
	s.SetSolid(mask)
	return s
}

// TestFusedMatchesLegacy is the gate for the fused kernel: identical
// populations, macroscopic fields and forces, across body shapes, worker
// counts and both materialization modes.
func TestFusedMatchesLegacy(t *testing.T) {
	shapes := map[string]func() *Solver{
		"thin":      func() *Solver { return benchSolver(false) },
		"broadside": func() *Solver { return benchSolver(true) },
		"wall":      wallHugger,
		"outflow":   outflowHugger,
	}
	for name, mk := range shapes {
		for _, workers := range []int{1, 2, 3, 7} {
			for _, store := range []bool{true, false} {
				t.Run(name, func(t *testing.T) {
					ref := mk()
					got := mk()
					got.Workers = workers

					// Several steps so any drift accumulates instead of
					// hiding in the first one.
					for range 4 {
						ref.legacyStep(store)
						ref.f, ref.ftmp = ref.ftmp, ref.f

						got.fusedStep(store)
						got.f, got.ftmp = got.ftmp, got.f
					}

					for i := range 9 {
						for c := range ref.f[i] {
							if got.f[i][c] != ref.f[i][c] {
								t.Fatalf("%s workers=%d store=%v: f[%d][%d] = %v, want %v",
									name, workers, store, i, c, got.f[i][c], ref.f[i][c])
							}
						}
					}
					if !store {
						return
					}
					for c := range ref.Rho {
						if got.Rho[c] != ref.Rho[c] || got.Ux[c] != ref.Ux[c] || got.Uy[c] != ref.Uy[c] {
							t.Fatalf("%s workers=%d: macroscopic fields differ at cell %d", name, workers, c)
						}
					}
				})
			}
		}
	}
}
