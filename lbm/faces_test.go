package lbm

import "testing"

// sweepForce is the original four-sweep force integral, kept as the reference
// the cached face list must match bit for bit.
func sweepForce(s *Solver) (fx, fy, mz, sep float64) {
	nx, ny := s.NX, s.NY
	surf, rev := 0, 0
	for _, j := range [4]int{1, 2, 3, 4} {
		for y := range ny {
			for x := range nx {
				c := y*nx + x
				if s.solid[c] {
					continue
				}
				xn := x + exi[j]
				yn := y + eyi[j]
				if xn < 0 || xn >= nx || yn < 0 || yn >= ny {
					continue
				}
				if !s.solid[yn*nx+xn] {
					continue
				}
				surf++
				if s.Ux[c] < 0 {
					rev++
				}
				p := s.Rho[c] / 3
				dfx := p * exf[j]
				dfy := p * eyf[j]
				px := float64(x) + 0.5*exf[j]
				py := float64(y) + 0.5*eyf[j]
				fx += dfx
				fy += dfy
				mz += px*dfy - py*dfx
			}
		}
	}
	if surf > 0 {
		sep = float64(rev) / float64(surf)
	}
	return fx, fy, mz, sep
}

// TestFaceListMatchesSweep pins the face-list computeForce to the original
// full-grid sweep, bit for bit, on both body shapes and after an UpdateSolid.
func TestFaceListMatchesSweep(t *testing.T) {
	for _, broadside := range []bool{false, true} {
		s := benchSolver(broadside)
		for range 5 {
			s.Step()
		}
		fx, fy, mz, sep := sweepForce(s)
		if s.Fx != fx || s.Fy != fy || s.Mz != mz || s.Sep != sep {
			t.Errorf("broadside=%v: faces gave F=(%v,%v) Mz=%v Sep=%v, sweep gave F=(%v,%v) Mz=%v Sep=%v",
				broadside, s.Fx, s.Fy, s.Mz, s.Sep, fx, fy, mz, sep)
		}
	}

	// A mask swapped in through UpdateSolid must rebuild the list too.
	s := benchSolver(false)
	mask := make([]bool, s.NX*s.NY)
	for y := 90; y < 110; y++ {
		for x := 100; x < 220; x++ {
			mask[y*s.NX+x] = true
		}
	}
	s.UpdateSolid(mask)
	s.Step()
	fx, fy, mz, sep := sweepForce(s)
	if s.Fx != fx || s.Fy != fy || s.Mz != mz || s.Sep != sep {
		t.Errorf("after UpdateSolid: faces F=(%v,%v) Mz=%v Sep=%v, sweep F=(%v,%v) Mz=%v Sep=%v",
			s.Fx, s.Fy, s.Mz, s.Sep, fx, fy, mz, sep)
	}
}
