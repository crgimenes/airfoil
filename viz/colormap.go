// Package viz holds the rendering-agnostic pieces of the visualization: color
// maps for the scalar fields and the smoke-tracer particle system. Keeping them
// free of any Ebiten dependency lets them be unit-tested headlessly; the app
// layer turns their output into pixels.
package viz

import (
	"image/color"
	"math"
)

// speedStops is a perceptually rising ramp (deep blue → cyan → green → yellow →
// red) used to paint flow speed, echoing the streamline plots in the references.
var speedStops = []color.RGBA{
	{0x05, 0x10, 0x40, 0xff},
	{0x10, 0x70, 0xc0, 0xff},
	{0x20, 0xc0, 0xa0, 0xff},
	{0xd0, 0xe0, 0x30, 0xff},
	{0xff, 0x30, 0x20, 0xff},
}

// Speed maps a normalized speed t in [0,1] to a color along the ramp.
func Speed(t float64) color.RGBA {
	return rampAt(speedStops, clamp01(t))
}

// Vorticity maps signed curl to a diverging map: counter-clockwise (positive)
// toward warm red, clockwise toward cool blue, near-zero left dark so the body
// and quiet free stream recede and the shed vortices pop. scale is the curl
// magnitude that saturates the color.
func Vorticity(v, scale float64) color.RGBA {
	if scale <= 0 {
		return color.RGBA{0x08, 0x0a, 0x12, 0xff}
	}
	t := clampSym(v / scale) // -1..1
	mag := math.Abs(t)
	base := color.RGBA{0x08, 0x0a, 0x12, 0xff}
	tip := color.RGBA{0x30, 0x90, 0xff, 0xff} // clockwise spin reads cool blue
	if t >= 0 {
		tip = color.RGBA{0xff, 0x60, 0x30, 0xff} // counter-clockwise reads warm
	}
	return lerpColor(base, tip, mag)
}

// Pressure colors of the pressure coefficient with a blue-white-red diverging
// map: high pressure (positive Cp, stagnation) reads red, suction (negative Cp)
// reads blue, ambient sits near white — the convention of textbook Cp plots.
// scale is the |Cp| that saturates the color.
var (
	pressureLow  = color.RGBA{0x20, 0x6c, 0xff, 0xff}
	pressureMid  = color.RGBA{0xec, 0xee, 0xf2, 0xff}
	pressureHigh = color.RGBA{0xff, 0x44, 0x2a, 0xff}
)

func Pressure(cp, scale float64) color.RGBA {
	if scale <= 0 {
		return pressureMid
	}
	t := clampSym(cp / scale)
	if t >= 0 {
		return lerpColor(pressureMid, pressureHigh, t)
	}
	return lerpColor(pressureMid, pressureLow, -t)
}

// rampAt linearly interpolates within a list of color stops at position t∈[0,1].
func rampAt(stops []color.RGBA, t float64) color.RGBA {
	if t <= 0 {
		return stops[0]
	}
	if t >= 1 {
		return stops[len(stops)-1]
	}
	scaled := t * float64(len(stops)-1)
	i := int(scaled)
	frac := scaled - float64(i)
	return lerpColor(stops[i], stops[i+1], frac)
}

func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: lerpByte(a.R, b.R, t),
		G: lerpByte(a.G, b.G, t),
		B: lerpByte(a.B, b.B, t),
		A: 0xff,
	}
}

func lerpByte(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

// clamp01 bounds t to [0,1]. NaN maps to 0: a NaN sails through the < and >
// comparisons, and downstream rampAt would turn it into int(NaN) — a huge
// negative slice index. The renderer must never be able to panic on a sick
// field value, whatever the solver produced.
func clamp01(t float64) float64 {
	if math.IsNaN(t) {
		return 0
	}
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

// clampSym bounds t to [-1,1]; NaN maps to 0 for the same reason as clamp01.
func clampSym(t float64) float64 {
	if math.IsNaN(t) {
		return 0
	}
	if t < -1 {
		return -1
	}
	if t > 1 {
		return 1
	}
	return t
}
