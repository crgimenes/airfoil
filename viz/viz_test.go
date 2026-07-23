package viz

import (
	"math"
	"testing"
)

// TestColormapsNaNSafe pins the fix for issue #1's crash: clamp01(NaN) used to
// stay NaN, and rampAt then indexed stops[int(NaN)] — a panic. Every colormap
// must swallow non-finite input and return a valid color.
func TestColormapsNaNSafe(t *testing.T) {
	nan := math.NaN()
	got := Speed(nan)
	if got != speedStops[0] {
		t.Errorf("Speed(NaN) = %+v, want first stop", got)
	}
	Vorticity(nan, 1) // must not panic
	Pressure(nan, 1)  // must not panic
	got = Speed(math.Inf(1))
	if got != speedStops[len(speedStops)-1] {
		t.Errorf("Speed(+Inf) = %+v, want last stop", got)
	}
}

func TestSpeedRampEndpoints(t *testing.T) {
	lo := Speed(-1) // clamps to 0
	hi := Speed(2)  // clamps to 1
	if lo != speedStops[0] {
		t.Errorf("Speed(0) = %+v, want first stop %+v", lo, speedStops[0])
	}
	if hi != speedStops[len(speedStops)-1] {
		t.Errorf("Speed(1) = %+v, want last stop", hi)
	}
}

func TestVorticityDivergingSign(t *testing.T) {
	pos := Vorticity(1, 1)
	neg := Vorticity(-1, 1)
	if pos.R <= pos.B {
		t.Errorf("positive curl should read warm: %+v", pos)
	}
	if neg.B <= neg.R {
		t.Errorf("negative curl should read cool: %+v", neg)
	}
	zero := Vorticity(0, 1)
	if zero.R > 0x20 || zero.B > 0x20 {
		t.Errorf("zero curl should be dark: %+v", zero)
	}
}

func TestPressureDivergingSign(t *testing.T) {
	high := Pressure(1, 1)
	low := Pressure(-1, 1)
	if high.R <= high.B {
		t.Errorf("high pressure should read warm: %+v", high)
	}
	if low.B <= low.R {
		t.Errorf("suction should read cool: %+v", low)
	}
	if Pressure(0, 1) != pressureMid {
		t.Errorf("ambient pressure should be the neutral color: %+v", Pressure(0, 1))
	}
}

// fakeField is a uniform rightward flow with an optional solid column, used to
// drive the tracer logic deterministically.
type fakeField struct {
	ux, uy   float64
	solidCol int // cells with x == solidCol are solid; <0 disables
}

func (f fakeField) VelocityAt(x, y float64) (float64, float64) { return f.ux, f.uy }
func (f fakeField) Solid(x, y int) bool                        { return f.solidCol >= 0 && x == f.solidCol }

func TestParticlesAdvectAndRecycle(t *testing.T) {
	f := fakeField{ux: 1, uy: 0, solidCol: -1}
	p := NewParticles(50, 100, 40, 1)
	startSum := 0.0
	for _, x := range p.X {
		startSum += x
	}
	for range 10 {
		p.Step(f, 1)
	}
	// With rightward flow every tracer must have moved downstream on average.
	endSum := 0.0
	for _, x := range p.X {
		endSum += x
		if x < 0 || x >= 100 {
			t.Fatalf("tracer left the domain without recycling: x=%g", x)
		}
	}
	if endSum <= startSum {
		t.Errorf("tracers did not advance: start=%g end=%g", startSum, endSum)
	}
}

func TestParticlesRespawnOutOfSolid(t *testing.T) {
	f := fakeField{ux: 0, uy: 0, solidCol: 5}
	p := NewParticles(20, 100, 40, 2)
	// Force one tracer into the solid column and step; it must be respawned out.
	p.X[0], p.Y[0] = 5.5, 20
	p.Step(f, 1)
	if f.Solid(int(p.X[0]), int(p.Y[0])) {
		t.Errorf("tracer stayed inside the body: x=%g y=%g", p.X[0], p.Y[0])
	}
}

func TestSeedYWithinBand(t *testing.T) {
	ny := 40
	p := NewParticles(1, 100, ny, 3)
	for range 2000 {
		y := p.seedY()
		if y < 1 || y >= float64(ny-1) {
			t.Fatalf("seedY out of valid band: %g (ny=%d)", y, ny)
		}
	}
}
