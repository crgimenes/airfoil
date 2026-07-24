//go:build !js

package lbm

import (
	"runtime"
	"sync"
)

// forRows runs fn over [0, ny) split into contiguous row bands, one goroutine
// per band. Both phases that use it (collide, the interior of stream) write
// each cell from that cell's own inputs alone, so the split changes nothing
// but the wall clock; results are bit-identical for any worker count.
//
// The goroutines are spawned per call rather than pooled: a phase costs
// hundreds of microseconds and a spawn about one, so a pool would buy noise.
func (s *Solver) forRows(ny int, fn func(y0, y1 int)) {
	workers := s.workerCount(ny)
	if workers <= 1 {
		fn(0, ny)
		return
	}
	chunk := (ny + workers - 1) / workers
	var wg sync.WaitGroup
	for w := range workers {
		y0 := w * chunk
		y1 := min(y0+chunk, ny)
		if y0 >= y1 {
			break
		}
		wg.Go(func() {
			fn(y0, y1)
		})
	}
	wg.Wait()
}

// forRowsIdx is forRows with the band index handed to fn, for phases that need
// per-worker scratch. The index is dense and below the worker count, so it can
// address a preallocated slice.
func (s *Solver) forRowsIdx(ny int, fn func(w, y0, y1 int)) {
	workers := s.workerCount(ny)
	if workers <= 1 {
		fn(0, 0, ny)
		return
	}
	chunk := (ny + workers - 1) / workers
	var wg sync.WaitGroup
	for w := range workers {
		y0 := w * chunk
		y1 := min(y0+chunk, ny)
		if y0 >= y1 {
			break
		}
		wg.Go(func() {
			fn(w, y0, y1)
		})
	}
	wg.Wait()
}

// workerCount is the band count both row helpers agree on.
func (s *Solver) workerCount(ny int) int {
	workers := s.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	return min(workers, ny)
}
