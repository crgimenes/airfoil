//go:build js

package lbm

// forRows on wasm is a plain serial call: the browser build runs on one
// thread, so goroutines here would add scheduling without adding hardware.
func (s *Solver) forRows(ny int, fn func(y0, y1 int)) {
	fn(0, ny)
}
