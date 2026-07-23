package viz

import (
	"math"
	"math/rand"
)

// Field is the slice of the solver the particle system needs: a velocity sample
// at any continuous grid position and a solid test on integer cells.
type Field interface {
	VelocityAt(x, y float64) (ux, uy float64)
	Solid(x, y int) bool
}

// Particles are passive smoke tracers carried by the flow. They are seeded along
// the inlet and recycled when they leave the domain, stall in a recirculation
// bubble, or wander into the body, which keeps a steady set of streaklines like
// the smoke ribbons in a real tunnel.
type Particles struct {
	X, Y []float64
	// Spd is each tracer's flow speed from the advection sample of the last
	// Step, so the renderer can tint without re-interpolating the field.
	Spd []float64
	nx  int
	ny  int
	rng *rand.Rand
}

// NewParticles builds count tracers for an nx×ny grid. seed fixes the RNG so
// runs are reproducible (and tests deterministic).
func NewParticles(count, nx, ny int, seed int64) *Particles {
	p := &Particles{
		X:   make([]float64, count),
		Y:   make([]float64, count),
		Spd: make([]float64, count),
		nx:  nx,
		ny:  ny,
		rng: rand.New(rand.NewSource(seed)), // #nosec G404 -- cosmetic seeding, not security
	}
	for i := range p.X {
		// Scatter the initial cloud across the whole domain, not just the inlet:
		// a clump seeded at the inlet would drift and recycle in lock-step,
		// arriving in visible pulsing waves. Spreading them desynchronizes the
		// recycling so the streaklines look steady from the first frame. Tracers
		// that start inside the body are cleared on the first Step by recycle.
		p.X[i] = p.rng.Float64() * float64(nx-1)
		p.Y[i] = 1 + p.rng.Float64()*float64(ny-2)
	}
	return p
}

// Step advects every tracer one frame. dt scales the lattice velocity into
// grid cells per frame (a few cells looks lively without skipping past detail).
// Tracers that exit, stall, or enter the body are respawned at the inlet.
func (p *Particles) Step(f Field, dt float64) {
	for i := range p.X {
		ux, uy := f.VelocityAt(p.X[i], p.Y[i])
		p.X[i] += ux * dt
		p.Y[i] += uy * dt
		p.Spd[i] = math.Sqrt(ux*ux + uy*uy)
		if p.recycle(f, i, ux, uy) {
			p.respawn(f, i)
		}
	}
}

// recycle reports whether tracer i should be reseeded: it left the domain, hit
// the body, stalled in a dead-water region, or went non-finite.
func (p *Particles) recycle(f Field, i int, ux, uy float64) bool {
	x, y := p.X[i], p.Y[i]
	if math.IsNaN(x) || math.IsNaN(y) {
		return true
	}
	if x < 0 || x >= float64(p.nx-1) || y < 1 || y >= float64(p.ny-1) {
		return true
	}
	if f.Solid(int(x), int(y)) {
		return true
	}
	// A tracer that has crept past the body but barely moves is trapped in a
	// recirculation bubble; recycle it so streaklines stay fresh.
	return ux*ux+uy*uy < 1e-8
}

// respawn reseeds tracer i at the inlet, avoiding the body.
func (p *Particles) respawn(f Field, i int) {
	for range 8 {
		x := p.rng.Float64() * 2
		y := p.seedY()
		if !f.Solid(int(x), int(y)) {
			p.X[i], p.Y[i] = x, y
			return
		}
	}
	p.X[i], p.Y[i] = 0.5, float64(p.ny)/2
}

// seedY picks an inlet y, biased toward the vertical centre band where the body
// usually sits, so more streaklines flow around it (denser smoke near the foil).
func (p *Particles) seedY() float64 {
	// A gentle bias: a third of respawns land in the centre band (denser smoke
	// around the body); the rest stay uniform so the whole tunnel stays filled.
	if p.rng.Float64() < 0.35 {
		c := float64(p.ny) / 2
		y := c + (p.rng.Float64()-0.5)*float64(p.ny)/3 // centre +- ny/6
		return math.Max(1, math.Min(float64(p.ny-2), y))
	}
	return 1 + p.rng.Float64()*float64(p.ny-2)
}
