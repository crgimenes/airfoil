//go:build js

package lbm

// stepOnce advances one lattice step using the separate collide, boundary and
// stream phases.
//
// The browser build deliberately keeps this path: the fused kernel that the
// native builds use measured 85% SLOWER here (25.1 ms against 13.5 ms per
// frame). It walks the grid a row at a time, so it trades three long loops for
// several hundred short ones, and the wasm JIT optimizes the long loops far
// better than it recovers the call and setup overhead of the short ones. The
// memory traffic the fusion saves is not the constraint inside the VM.
func (s *Solver) stepOnce(store bool) {
	s.collide(store)
	s.applyBoundaries()
	s.stream()
}
