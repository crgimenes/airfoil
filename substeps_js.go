//go:build js

package main

// substeps is how many solver steps advance per displayed frame.
//
// WebAssembly runs the solver about 2.6x slower than a native build (measured
// on this grid: 6.1 ms a step against 2.3 ms). Three steps would need 18 ms,
// which overruns a 60 Hz frame before anything is drawn, and the browser build
// stutters. One step leaves the frame comfortable.
//
// The trade is that the flow evolves slower in wall-clock time than on the
// desktop. Smooth and slow reads better than fast and juddering, and the
// physics per step is identical either way.
//
// It's a var, not a const, so the same -substeps flag in main.go that
// overrides the native default can override this one too.
var substeps = 1
