package lbm

import "testing"

// TestStepNMatchesRepeatedStep pins the batched step to the one-at-a-time form
// it replaces. StepN skips the macroscopic fields on intermediate substeps, so
// the guarantee is that the populations, the published fields and the forces
// all come out bit-identical once the batch is done.
func TestStepNMatchesRepeatedStep(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5} {
		ref := benchSolver(false)
		for range n {
			ref.Step()
		}

		got := benchSolver(false)
		got.StepN(n)

		for i := range 9 {
			for c := range ref.f[i] {
				if got.f[i][c] != ref.f[i][c] {
					t.Fatalf("n=%d: f[%d][%d] = %v, want %v", n, i, c, got.f[i][c], ref.f[i][c])
				}
			}
		}
		for c := range ref.Rho {
			if got.Rho[c] != ref.Rho[c] || got.Ux[c] != ref.Ux[c] || got.Uy[c] != ref.Uy[c] {
				t.Fatalf("n=%d: macroscopic fields differ at cell %d", n, c)
			}
		}
		if got.Fx != ref.Fx || got.Fy != ref.Fy || got.Mz != ref.Mz || got.Sep != ref.Sep {
			t.Fatalf("n=%d: forces differ: got F=(%v,%v) Mz=%v Sep=%v, want F=(%v,%v) Mz=%v Sep=%v",
				n, got.Fx, got.Fy, got.Mz, got.Sep, ref.Fx, ref.Fy, ref.Mz, ref.Sep)
		}
	}
}
