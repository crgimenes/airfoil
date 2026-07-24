package lbm

import "testing"

// benchSolver builds the solver in the two shapes that bound the wall paths: a
// slender plate (few faces, the normal case) or a broadside slab (many faces
// and heavy blocking, the stress case).
func benchSolver(broadside bool) *Solver {
	s := New(360, 200, 0.6, 0.10)
	mask := make([]bool, 360*200)
	x0, x1, y0, y1 := 90, 235, 95, 105
	if broadside {
		x0, x1, y0, y1 = 155, 170, 30, 170
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			mask[y*360+x] = true
		}
	}
	s.SetSolid(mask)
	// A few steps so the fields hold a developed flow, not the uniform seed.
	for range 10 {
		s.Step()
	}
	return s
}

func BenchmarkStepThin(b *testing.B) {
	s := benchSolver(false)
	b.ReportAllocs()
	for b.Loop() {
		s.Step()
	}
}

func BenchmarkStepBroadside(b *testing.B) {
	s := benchSolver(true)
	b.ReportAllocs()
	for b.Loop() {
		s.Step()
	}
}

// BenchmarkFrame is the per-frame solver cost as the app pays it: the desktop
// substep count plus the Finite backstop.
func BenchmarkFrame(b *testing.B) {
	s := benchSolver(false)
	b.ReportAllocs()
	for b.Loop() {
		s.StepN(3)
		s.Finite()
	}
}

func BenchmarkCollide(b *testing.B) {
	s := benchSolver(false)
	b.ReportAllocs()
	for b.Loop() {
		s.collide(true)
	}
}

func BenchmarkStream(b *testing.B) {
	s := benchSolver(false)
	b.ReportAllocs()
	for b.Loop() {
		s.stream()
	}
}

func BenchmarkComputeForceThin(b *testing.B) {
	s := benchSolver(false)
	b.ReportAllocs()
	for b.Loop() {
		s.computeForce()
	}
}

func BenchmarkComputeForceBroadside(b *testing.B) {
	s := benchSolver(true)
	b.ReportAllocs()
	for b.Loop() {
		s.computeForce()
	}
}

func BenchmarkApplyBoundaries(b *testing.B) {
	s := benchSolver(false)
	b.ReportAllocs()
	for b.Loop() {
		s.applyBoundaries()
	}
}

func BenchmarkUpdateSolid(b *testing.B) {
	s := benchSolver(false)
	mask := make([]bool, 360*200)
	for y := 95; y < 105; y++ {
		for x := 91; x < 236; x++ {
			mask[y*360+x] = true
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		s.UpdateSolid(mask)
	}
}
