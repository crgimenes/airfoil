// Package lbm is a 2D Lattice-Boltzmann fluid solver (D2Q9, single-relaxation
// BGK) for the wind-tunnel simulation. It is deliberately qualitative, not a
// validated CFD code: lattice units stand in for physical ones, so it captures
// the *shape* of the flow (stagnation point, wake, vortex shedding, separation
// at high angle of attack) rather than calibrated forces.
//
// The package is pure Go with no rendering dependency so it can be unit-tested
// headlessly. Coordinates use x to the right and y up; the inlet is the left
// edge with a steady rightward free stream.
package lbm

import "math"

// D2Q9 lattice: a rest population plus 4 axial and 4 diagonal neighbors.
// Directions are ordered 0:rest, 1:E, 2:N, 3:W, 4:S, 5:NE, 6:NW, 7:SW, 8:SE.
var (
	exi = [9]int{0, 1, 0, -1, 0, 1, -1, -1, 1}
	eyi = [9]int{0, 0, 1, 0, -1, 1, 1, -1, -1}
	exf = [9]float64{0, 1, 0, -1, 0, 1, -1, -1, 1}
	eyf = [9]float64{0, 0, 1, 0, -1, 1, 1, -1, -1}
	// w are the D2Q9 equilibrium weights; they sum to 1.
	w = [9]float64{
		4.0 / 9.0,
		1.0 / 9.0, 1.0 / 9.0, 1.0 / 9.0, 1.0 / 9.0,
		1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0,
	}
	// opp maps each direction to the one pointing the opposite way, used by the
	// no-slip bounce-back at solid walls.
	opp = [9]int{0, 3, 4, 1, 2, 7, 8, 5, 6}
)

// Stability-clamp bounds for the collide step. uMax sits well above anything the
// app's normal envelope produces (local peaks stay under ~0.2 at the top inlet
// speed on a slender body) and well below where the D2Q9 equilibrium turns
// negative, so it only engages in blow-up territory.
const (
	uMax   = 0.35
	rhoMin = 0.5
	rhoMax = 2.0
)

// Solver holds the populations and the derived macroscopic fields for one grid.
type Solver struct {
	NX, NY int

	// Workers caps the goroutines the native build spreads collide and stream
	// across; 0 means one per available CPU. The browser build is always
	// serial. Any value produces bit-identical results, since both phases
	// write each cell from that cell's own inputs.
	Workers int

	omega float64 // BGK relaxation rate, 1/tau
	u0    float64 // free-stream speed in lattice units, along +x

	// Populations are float32 to halve the solver's memory footprint and traffic;
	// the kernel is memory-bound, so this is the lever, not more arithmetic. All
	// math is still done in float64 (promote on read, demote on write), so only
	// storage precision drops. Macroscopic fields stay float64 for the API.
	f, ftmp [9][]float32 // distribution functions, double-buffered for streaming
	solid   []bool       // true where a cell is part of the body (wall)

	// Macroscopic fields, refreshed every Step and indexed y*NX+x. Exposed for
	// visualization and particle advection; solid cells read as zero velocity.
	Rho, Ux, Uy []float64

	// Fx, Fy are the instantaneous force the fluid exerts on the body this step,
	// in lattice units. Fx is drag (with the stream), Fy is lift (across it). Mz
	// is the moment of that force about the grid origin (0,0), positive counter-
	// clockwise; the app uses it to locate the center of pressure.
	Fx, Fy, Mz float64

	// Sep is the fraction of the body's surface faces where the adjacent flow has
	// reversed (streamwise velocity < 0), updated every Step. Attached flow keeps
	// it near zero; a separation bubble (post-stall) drives it up, so it serves as
	// a qualitative stall indicator independent of the lift curve.
	Sep float64

	// faces caches the body's exposed faces so computeForce visits only the
	// surface instead of sweeping the grid four times per step. Rebuilt whenever
	// the solid mask changes, in the same j,y,x order as the sweep it replaced,
	// so the floating-point sums come out bit-identical.
	faces []face

	// faceBins are per-direction scratch lists that let rebuildFaces scan the
	// grid once while still emitting the list in j-major order. An animated
	// scene rebuilds every frame, so the scan count matters.
	faceBins [4][]face

	// rings is the fused kernel's per-worker row window; see fused.go. The
	// browser build steps through the separate phases instead (step_js.go), so
	// there the field stays nil and unread -- a few words of struct against
	// splitting the type in two across build tags.
	//nolint:unused // only the native build's fused kernel fills this
	rings [][]float32
}

// face is one exposed body face: a fluid cell whose axial neighbor is solid.
type face struct {
	c      int     // fluid cell index
	ex, ey float64 // face direction, pointing into the body
	px, py float64 // face midpoint, the moment arm for Mz
}

// New builds a solver of the given size. tau is the BGK relaxation time
// (viscosity nu = (tau-0.5)/3; keep tau > 0.5 for stability) and u0 is the
// inlet speed in lattice units (keep well below the lattice sound speed ~0.577,
// e.g. <= 0.1, to stay near-incompressible). The field starts as uniform flow.
func New(nx, ny int, tau, u0 float64) *Solver {
	s := &Solver{
		NX:    nx,
		NY:    ny,
		omega: 1.0 / tau,
		u0:    u0,
		solid: make([]bool, nx*ny),
		Rho:   make([]float64, nx*ny),
		Ux:    make([]float64, nx*ny),
		Uy:    make([]float64, nx*ny),
	}
	for i := range 9 {
		s.f[i] = make([]float32, nx*ny)
		s.ftmp[i] = make([]float32, nx*ny)
	}
	s.Reset()
	return s
}

// Reset re-initializes every cell to the uniform free stream. Call it after the
// body geometry changes so stale wake structures do not linger.
func (s *Solver) Reset() {
	for c := range s.Rho {
		s.Rho[c] = 1
		s.Ux[c] = s.u0
		s.Uy[c] = 0
		for i := range 9 {
			s.f[i][c] = float32(feq(i, 1, s.u0, 0))
		}
	}
}

// SetSolid replaces the body mask. The slice must have NX*NY entries laid out as
// y*NX+x. The flow field is reset so the new body starts from clean inflow; use
// it for a big geometry change (a different profile) where a lingering wake would
// look wrong.
func (s *Solver) SetSolid(mask []bool) {
	copy(s.solid, mask)
	s.rebuildFaces()
	s.Reset()
}

// UpdateSolid swaps in a new body mask without resetting the flow, so small
// changes (nudging the angle of attack) evolve smoothly instead of restarting.
// Cells that turn from solid into fluid are seeded with the free-stream
// equilibrium, since their populations went stale while they were walls.
func (s *Solver) UpdateSolid(mask []bool) {
	for c := range mask {
		if s.solid[c] && !mask[c] {
			for i := range 9 {
				s.f[i][c] = float32(feq(i, 1, s.u0, 0))
			}
		}
		s.solid[c] = mask[c]
	}
	s.rebuildFaces()
}

// SetInletSpeed changes the free-stream speed in place. The boundaries pick up
// the new value on the next step, so the flow accelerates smoothly without a
// reset.
func (s *Solver) SetInletSpeed(u0 float64) {
	s.u0 = u0
}

// Solid reports whether the cell at (x,y) belongs to the body.
func (s *Solver) Solid(x, y int) bool {
	return s.solid[y*s.NX+x]
}

// Finite reports whether every macroscopic field and integral force is a real
// number. The collide clamp should keep this always true; the app still checks
// it after stepping as a backstop, so a blow-up in some unforeseen corner
// resets the flow instead of feeding NaN to the renderer.
func (s *Solver) Finite() bool {
	for c := range s.Rho {
		if !finite(s.Rho[c]) || !finite(s.Ux[c]) || !finite(s.Uy[c]) {
			return false
		}
	}
	return finite(s.Fx) && finite(s.Fy) && finite(s.Mz) && finite(s.Sep)
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// feq is the discrete equilibrium distribution for direction i given the local
// density and velocity.
func feq(i int, rho, ux, uy float64) float64 {
	cu := exf[i]*ux + eyf[i]*uy
	usq := ux*ux + uy*uy
	return w[i] * rho * (1 + 3*cu + 4.5*cu*cu - 1.5*usq)
}

// Step advances the simulation by one lattice time unit: collide, apply the
// open-channel boundaries, tally the wall force, then stream with bounce-back.
func (s *Solver) Step() {
	s.StepN(1)
}

// StepN advances n lattice time units.
//
// Only the last of the n steps materializes the macroscopic fields and the
// wall forces. Nothing outside the solver can observe the intermediate values:
// Fx/Fy/Mz/Sep are overwritten by the next step, and Rho/Ux/Uy are read by the
// renderer and the tracers only after the whole batch is done. Writing three
// full-grid fields per substep is therefore pure memory traffic, and on a
// memory-bound kernel traffic is the cost.
func (s *Solver) StepN(n int) {
	for i := range n {
		last := i == n-1
		s.stepOnce(last)
		if last {
			s.computeForce()
		}
		s.f, s.ftmp = s.ftmp, s.f
	}
}

// collide relaxes every fluid cell toward local equilibrium (BGK) and refreshes
// the macroscopic fields in place. Solid cells are skipped and report no flow.
//
// This phase and the two that follow it are the browser build's step (see
// step_js.go) and the reference the fused kernel is tested against, so a
// deadcode run over a native build reports them unreachable. They are not.
func (s *Solver) collide(store bool) {
	s.forRows(s.NY, func(y0, y1 int) {
		s.collideRange(y0*s.NX, y1*s.NX, store)
	})
}

// collideRange is collide over the cell range [c0, c1), the unit forRows
// hands to each worker. store says whether to publish the macroscopic fields;
// see StepN for why an intermediate substep skips them.
func (s *Solver) collideRange(c0, c1 int, store bool) {
	for c := c0; c < c1; c++ {
		if s.solid[c] {
			if store {
				s.Rho[c], s.Ux[c], s.Uy[c] = 1, 0, 0
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
		// Stability net: extreme settings (a broadside body at the top inlet
		// speed blocks ~70% of the channel) push local speeds past what BGK at
		// this tau can integrate, and the run dissolves into NaNs. Clamping the
		// macroscopic inputs of the equilibrium keeps the populations finite; in
		// the normal envelope (|u| stays below ~0.2) the clamp is inert, so the
		// physics elsewhere is untouched.
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
		// The equilibrium's 1 - 1.5*u^2 term is the same for all nine
		// directions, so it is computed once here instead of inside feq nine
		// times. Same math, fewer multiplies; forces stay within the physics
		// tolerances (the reassociation shifts the last bits only).
		base := 1 - 1.5*(ux*ux+uy*uy)
		wrho := rho * s.omega
		for i := range 9 {
			cu := exf[i]*ux + eyf[i]*uy
			fi := float64(s.f[i][c])
			s.f[i][c] = float32(fi + wrho*w[i]*(base+3*cu+4.5*cu*cu) - s.omega*fi)
		}
	}
}

// applyBoundaries models an open channel: the left, top and bottom edges are
// pinned to the free stream, and the right edge is a zero-gradient outflow.
// Writing these into the post-collision field lets streaming carry them inward.
func (s *Solver) applyBoundaries() {
	nx, ny := s.NX, s.NY
	// Every free-stream boundary cell gets the same nine equilibrium values,
	// so they are computed once per call instead of once per cell.
	var eq [9]float32
	for i := range 9 {
		eq[i] = float32(feq(i, 1, s.u0, 0))
	}
	for y := range ny {
		c := y * nx
		for i := range 9 {
			s.f[i][c] = eq[i] // inlet
		}
		out := y*nx + (nx - 1)
		prev := y*nx + (nx - 2)
		for i := range 9 {
			s.f[i][out] = s.f[i][prev] // outflow: copy interior neighbor
		}
	}
	for x := range nx {
		bottom := x
		top := (ny-1)*nx + x
		for i := range 9 {
			s.f[i][bottom] = eq[i]
			s.f[i][top] = eq[i]
		}
	}
}

// computeForce integrates pressure over the body surface: every fluid cell that
// borders a solid cell across an axis pushes on that face with pressure p = rho/3
// (the lattice equation of state, cs^2 = 1/3) directed into the body. Summing
// over all such faces gives the net force the flow exerts on the body, with
// positive drag along the stream and positive lift across it.
//
// This is pressure (form) drag and lift only; skin friction is neglected, which
// is fine for a qualitative tool and is where pressure forces dominate anyway
// (stalled or bluff bodies). The momentum-exchange method would also fold in
// viscous stress, but it proved numerically noisy at this resolution, so the
// transparent pressure integral is preferred here.
func (s *Solver) computeForce() {
	fx, fy, mz := 0.0, 0.0, 0.0
	rev := 0
	for _, fc := range s.faces {
		if s.Ux[fc.c] < 0 {
			rev++
		}
		p := s.Rho[fc.c] / 3
		dfx := p * fc.ex
		dfy := p * fc.ey
		fx += dfx
		fy += dfy
		mz += fc.px*dfy - fc.py*dfx
	}
	s.Fx, s.Fy, s.Mz = fx, fy, mz
	s.Sep = 0
	if len(s.faces) > 0 {
		s.Sep = float64(rev) / float64(len(s.faces))
	}
}

// rebuildFaces rescans the mask for exposed faces. One scan per mask change
// replaces the four full-grid sweeps computeForce used to pay per step (twelve
// per desktop frame); even an animated scene changing the mask every frame
// comes out ahead. The j,y,x order mirrors the old sweep so the force sums are
// bit-identical.
func (s *Solver) rebuildFaces() {
	nx, ny := s.NX, s.NY
	for j := range s.faceBins {
		s.faceBins[j] = s.faceBins[j][:0]
	}
	// One scan over the grid, binned per direction so the final list keeps the
	// j-major order of the sweep this replaced (the float sums depend on it).
	// Only the four axial neighbors are real cell faces; diagonals are corners.
	for y := range ny {
		for x := range nx {
			c := y*nx + x
			if s.solid[c] {
				continue
			}
			for bin, j := range [4]int{1, 2, 3, 4} {
				xn := x + exi[j]
				yn := y + eyi[j]
				if xn < 0 || xn >= nx || yn < 0 || yn >= ny {
					continue
				}
				if !s.solid[yn*nx+xn] {
					continue
				}
				s.faceBins[bin] = append(s.faceBins[bin], face{
					c:  c,
					ex: exf[j],
					ey: eyf[j],
					// The face force acts at the face midpoint, half a cell
					// toward the solid, so the moment arm sits on the surface.
					px: float64(x) + 0.5*exf[j],
					py: float64(y) + 0.5*eyf[j],
				})
			}
		}
	}
	s.faces = s.faces[:0]
	for j := range s.faceBins {
		s.faces = append(s.faces, s.faceBins[j]...)
	}
}

// stream pulls each population from its upstream neighbor into the double buffer.
// A population whose source is a solid cell is reflected back (no-slip half-way
// bounce-back); a source outside the domain keeps the boundary value already set.
func (s *Solver) stream() {
	nx, ny := s.NX, s.NY
	// Interior cells, direction-major: every source is in-domain, so the four
	// bounds tests the general loop pays per cell per direction disappear, and
	// each direction walks two contiguous arrays. Assignments are identical to
	// the general loop's, so the populations come out bit for bit the same.
	solid := s.solid
	s.forRows(ny-2, func(b0, b1 int) {
		for i := range 9 {
			fi := s.f[i]
			fti := s.ftmp[i]
			fo := s.f[opp[i]]
			off := eyi[i]*nx + exi[i]
			for y := 1 + b0; y < 1+b1; y++ {
				row := y * nx
				for x := 1; x < nx-1; x++ {
					c := row + x
					if solid[c] {
						fti[c] = fi[c]
						continue
					}
					src := c - off
					if solid[src] {
						fti[c] = fo[c]
						continue
					}
					fti[c] = fi[src]
				}
			}
		}
	})
	// Border cells keep the general form, including the keep-boundary-value
	// case for sources outside the domain.
	for y := range ny {
		s.streamCell(0, y)
		s.streamCell(nx-1, y)
	}
	for x := 1; x < nx-1; x++ {
		s.streamCell(x, 0)
		s.streamCell(x, ny-1)
	}
}

// streamCell streams one cell with full bounds handling; only the domain
// border pays this general form.
func (s *Solver) streamCell(x, y int) {
	nx, ny := s.NX, s.NY
	c := y*nx + x
	if s.solid[c] {
		for i := range 9 {
			s.ftmp[i][c] = s.f[i][c]
		}
		return
	}
	for i := range 9 {
		sx := x - exi[i]
		sy := y - eyi[i]
		if sx < 0 || sx >= nx || sy < 0 || sy >= ny {
			s.ftmp[i][c] = s.f[i][c]
			continue
		}
		src := sy*nx + sx
		if s.solid[src] {
			s.ftmp[i][c] = s.f[opp[i]][c]
			continue
		}
		s.ftmp[i][c] = s.f[i][src]
	}
}

// VelocityAt samples the macroscopic velocity at a continuous grid position with
// bilinear interpolation, clamped to the domain. It is used to advect tracers.
func (s *Solver) VelocityAt(x, y float64) (ux, uy float64) {
	nx, ny := s.NX, s.NY
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	maxX := float64(nx - 1)
	maxY := float64(ny - 1)
	if x > maxX {
		x = maxX
	}
	if y > maxY {
		y = maxY
	}
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := x0 + 1
	y1 := y0 + 1
	if x1 > nx-1 {
		x1 = nx - 1
	}
	if y1 > ny-1 {
		y1 = ny - 1
	}
	tx := x - float64(x0)
	ty := y - float64(y0)
	ux = bilerp(s.Ux, nx, x0, y0, x1, y1, tx, ty)
	uy = bilerp(s.Uy, nx, x0, y0, x1, y1, tx, ty)
	return ux, uy
}

func bilerp(field []float64, nx, x0, y0, x1, y1 int, tx, ty float64) float64 {
	a := field[y0*nx+x0]
	b := field[y0*nx+x1]
	c := field[y1*nx+x0]
	d := field[y1*nx+x1]
	top := a + (b-a)*tx
	bot := c + (d-c)*tx
	return top + (bot-top)*ty
}

// Vorticity returns the out-of-plane curl (d uy/dx - d ux/dy) at the given cell
// using central differences, with one-sided fallback at the borders. Positive
// values spin counter-clockwise. Solid cells return zero.
func (s *Solver) Vorticity(x, y int) float64 {
	nx, ny := s.NX, s.NY
	c := y*nx + x
	if s.solid[c] {
		return 0
	}
	xm := x - 1
	xp := x + 1
	ym := y - 1
	yp := y + 1
	if xm < 0 {
		xm = 0
	}
	if xp > nx-1 {
		xp = nx - 1
	}
	if ym < 0 {
		ym = 0
	}
	if yp > ny-1 {
		yp = ny - 1
	}
	duydx := (s.Uy[y*nx+xp] - s.Uy[y*nx+xm]) / float64(xp-xm)
	duxdy := (s.Ux[yp*nx+x] - s.Ux[ym*nx+x]) / float64(yp-ym)
	return duydx - duxdy
}
