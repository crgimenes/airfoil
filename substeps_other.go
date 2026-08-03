//go:build !js

package main

// substeps is how many solver steps advance per displayed frame. A native build
// runs a step in about 2.3 ms on this grid, so three of them fit inside a 60 Hz
// frame with room left for the smoke, the field and the panels.
//
// It's a var, not a const: the -substeps flag in main.go overrides it, to
// trade physical accuracy/responsiveness for CPU headroom on slower hardware
// (e.g. a Raspberry Pi) without a rebuild.
var substeps = 3
