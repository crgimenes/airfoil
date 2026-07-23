package lbm

import "testing"

// TestParallelMatchesSerial pins the banded solver to the serial one, bit for
// bit, across worker counts including one that does not divide the row count.
func TestParallelMatchesSerial(t *testing.T) {
	ref := benchSolver(false)
	ref.Workers = 1
	for range 20 {
		ref.Step()
	}

	for _, workers := range []int{2, 3, 8} {
		s := benchSolver(false)
		s.Workers = workers
		for range 20 {
			s.Step()
		}
		for i := range 9 {
			for c := range ref.f[i] {
				if s.f[i][c] != ref.f[i][c] {
					t.Fatalf("workers=%d: f[%d][%d] = %v, serial %v", workers, i, c, s.f[i][c], ref.f[i][c])
				}
			}
		}
		if s.Fx != ref.Fx || s.Fy != ref.Fy || s.Mz != ref.Mz || s.Sep != ref.Sep {
			t.Fatalf("workers=%d: forces diverged from serial", workers)
		}
	}
}

func BenchmarkStepParallel(b *testing.B) {
	s := benchSolver(false)
	b.ReportAllocs()
	for b.Loop() {
		s.Step()
	}
}

func BenchmarkStepSerial1Worker(b *testing.B) {
	s := benchSolver(false)
	s.Workers = 1
	b.ReportAllocs()
	for b.Loop() {
		s.Step()
	}
}

func BenchmarkStep4Workers(b *testing.B) {
	s := benchSolver(false)
	s.Workers = 4
	b.ReportAllocs()
	for b.Loop() {
		s.Step()
	}
}
