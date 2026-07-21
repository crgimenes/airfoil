//go:build !js

package main

// substeps is how many solver steps advance per displayed frame. A native build
// runs a step in about 2.3 ms on this grid, so three of them fit inside a 60 Hz
// frame with room left for the smoke, the field and the panels.
const substeps = 3
