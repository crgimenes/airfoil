//go:build !js

package lbm

import "math"

// ringRows is how many collided rows the fused kernel keeps live. Streaming a
// row reads its own row and the two around it, so three is the whole window.
const ringRows = 3

// fusedStep runs collision, the open-channel boundaries and streaming in a
// single pass over the grid.
//
// The collided populations have exactly one consumer: streaming reads them and
// then they are discarded on the buffer swap. Writing them across the full grid
// and reading them back costs two extra passes over 2.6 MB, and this kernel is
// memory-bound, so those passes are the cost rather than the arithmetic.
// Instead each worker keeps the three rows it currently needs in a small ring
// (about 39 KB, cache resident) and the collided values never reach RAM.
//
// Results are bit-identical to the collide/applyBoundaries/stream sequence it
// replaces: the same arithmetic in the same order per cell, only the storage in
// between is different. fused_test pins that against a reference copy.
func (s *Solver) fusedStep(store bool) {
	var eq [9]float32
	for i := range 9 {
		eq[i] = float32(feq(i, 1, s.u0, 0))
	}
	s.ensureRings()
	s.forRowsIdx(s.NY, func(w, y0, y1 int) {
		ring := s.rings[w]
		// Prime the window. A worker also collides the row above and below its
		// band, since streaming its edge rows reads them; recomputing two rows
		// per worker is cheaper than coordinating with the neighbouring band.
		for y := y0 - 1; y <= y0+1; y++ {
			s.collideRowInto(ring, y, store && y >= y0 && y < y1, &eq)
		}
		for y := y0; y < y1; y++ {
			s.streamRowFromRing(ring, y)
			// Pull the next row into the window before it is needed.
			next := y + 2
			if next <= y1 {
				s.collideRowInto(ring, next, store && next < y1, &eq)
			}
		}
	})
}

// ensureRings allocates one scratch ring per worker, before any goroutine
// starts so the slice itself is never written concurrently.
func (s *Solver) ensureRings() {
	n := s.workerCount(s.NY)
	if len(s.rings) >= n && (n == 0 || len(s.rings[0]) == ringRows*9*s.NX) {
		return
	}
	s.rings = make([][]float32, n)
	for i := range s.rings {
		s.rings[i] = make([]float32, ringRows*9*s.NX)
	}
}

// collideRowInto collides row y into the ring slot it owns, then applies the
// boundary substitution for that row. Rows outside the grid are ignored, so the
// caller can prime the window without bounds checks of its own.
//
// store says whether to publish this row's macroscopic fields; a worker only
// publishes the rows it owns, so the halo rows it recomputes stay silent and no
// cell is written twice.
func (s *Solver) collideRowInto(ring []float32, y int, store bool, eq *[9]float32) {
	nx, ny := s.NX, s.NY
	if y < 0 || y >= ny {
		return
	}
	base := (y % ringRows) * 9 * nx
	row := y * nx
	for x := range nx {
		c := row + x
		if s.solid[c] {
			if store {
				s.Rho[c], s.Ux[c], s.Uy[c] = 1, 0, 0
			}
			// Collision skips solid cells, so their populations carry over
			// untouched, exactly as the separate collide left them.
			for i := range 9 {
				ring[base+i*nx+x] = s.f[i][c]
			}
			continue
		}
		rho := 0.0
		mx := 0.0
		my := 0.0
		for i := range 9 {
			fi := float64(s.f[i][c])
			rho += fi
			mx += fi * exf[i]
			my += fi * eyf[i]
		}
		ux := mx / rho
		uy := my / rho
		if rho < rhoMin {
			rho = rhoMin
		}
		if rho > rhoMax {
			rho = rhoMax
		}
		sp2 := ux*ux + uy*uy
		if sp2 > uMax*uMax {
			k := uMax / math.Sqrt(sp2)
			ux *= k
			uy *= k
		}
		if store {
			s.Rho[c], s.Ux[c], s.Uy[c] = rho, ux, uy
		}
		eqBase := 1 - 1.5*(ux*ux+uy*uy)
		wrho := rho * s.omega
		for i := range 9 {
			cu := exf[i]*ux + eyf[i]*uy
			fi := float64(s.f[i][c])
			ring[base+i*nx+x] = float32(fi + wrho*w[i]*(eqBase+3*cu+4.5*cu*cu) - s.omega*fi)
		}
	}

	// Boundary substitution, in the order applyBoundaries used: the inlet and
	// outflow columns first, then the top and bottom rows, which is why a
	// corner ends up at free stream rather than at the outflow copy.
	if y == 0 || y == ny-1 {
		for i := range 9 {
			r := ring[base+i*nx : base+i*nx+nx]
			for x := range r {
				r[x] = eq[i]
			}
		}
		return
	}
	for i := range 9 {
		off := base + i*nx
		ring[off] = eq[i]               // inlet: pinned to the free stream
		ring[off+nx-1] = ring[off+nx-2] // outflow: zero gradient
	}
}

// streamRowFromRing writes row y of the next buffer, pulling each population
// from the ring: the neighbour it came from, or a bounce-back off a wall, or
// the row's own value where the source lies outside the grid.
//
// The walk is direction-major so the destination slice, the source row and the
// bounce-back row are all resolved once per direction instead of once per cell.
// A cell-major version of this loop measured barely faster than the unfused
// kernel: the saved memory traffic went straight back out as index arithmetic.
func (s *Solver) streamRowFromRing(ring []float32, y int) {
	nx, ny := s.NX, s.NY
	row := y * nx
	self := (y % ringRows) * 9 * nx
	solidRow := s.solid[row : row+nx]
	for i := range 9 {
		fti := s.ftmp[i][row : row+nx]
		selfI := ring[self+i*nx : self+i*nx+nx]
		selfOpp := ring[self+opp[i]*nx : self+opp[i]*nx+nx]

		sy := y - eyi[i]
		if sy < 0 || sy >= ny {
			// The whole row streams from itself: the source is off the grid.
			for x := range nx {
				fti[x] = selfI[x]
			}
			continue
		}
		src := ring[(sy%ringRows)*9*nx+i*nx:][:nx]
		srcSolid := s.solid[sy*nx : sy*nx+nx]
		ex := exi[i]
		for x := range nx {
			if solidRow[x] {
				fti[x] = selfI[x]
				continue
			}
			sx := x - ex
			if sx < 0 || sx >= nx {
				fti[x] = selfI[x]
				continue
			}
			if srcSolid[sx] {
				fti[x] = selfOpp[x]
				continue
			}
			fti[x] = src[sx]
		}
	}
}
