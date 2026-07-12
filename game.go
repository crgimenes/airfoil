package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/crgimenes/glaze/menu"
	ui "github.com/crgimenes/minigui"
	"github.com/crgimenes/native/filedialog"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"

	"kutta/foil"
	"kutta/lbm"
	"kutta/scene"
	"kutta/sceneio"
	"kutta/viz"
)

// Tunable defaults. The grid is kept modest so several solver steps fit in one
// 60Hz frame; tau and u0 sit in the stable, near-incompressible range.
const (
	gridW    = 360
	gridH    = 200 // taller than the chord so the foil is not boxed in by the walls
	pixScale = 3
	tau      = 0.6
	defaultU = 0.10
	spdMin   = 0.02 // inlet speed range where the solver stays stable
	spdMax   = 0.15

	substeps    = 3    // solver steps per displayed frame
	tracerSpeed = 5.0  // visual advection multiplier for smoke tracers
	nParticles  = 3000 // dense enough to fill the whole tunnel, not just the centre

	chordFrac = 0.40 // chord length as a fraction of grid width
	leadXFrac = 0.26 // leading-edge x position as a fraction of grid width
	pivotFrac = 0.25 // pitch axis along the chord: the quarter-chord / aero center

	vortScale = 0.06 // curl magnitude that saturates the vorticity color
	cpScale   = 1.5  // pressure coefficient that saturates the pressure color
	forceVisK = 60.0 // pixels per unit of lattice force, for the arrows
	cpVecK    = 6.0  // grid cells per unit Cp, for the surface pressure arrows

	// animDt advances the animation per frame. Deliberately slow so the loop
	// spans enough solver steps for the flow to respond to the moving surface;
	// a fast loop washes out the lift change.
	animDt = 1.0 / 150.0

	aoaMin = -20 // sweep range of the live Cl-alpha plot, in degrees
	aoaMax = 20
	// aoaLimit is the full angle-of-attack range the control allows, well past
	// the plot range so the foil can be turned broadside (90 deg) to the flow as
	// a what-if. Beyond the linear region this is a qualitative demo, not data.
	aoaLimit  = 90
	clPlotMin = -2.0 // Cl axis range of the plot
	clPlotMax = 2.0

	// Stall indicator thresholds on the surface separation fraction (lbm.Sep).
	// Calibrated on the default NACA 2412 at this grid/Re by an AoA sweep:
	// separation begins ~6 deg and grows to ~0.30 near 14 deg. This grid's low
	// Reynolds number keeps lift rising past separation, so the indicator tracks
	// separation extent (the cause of stall) rather than a lift-curve collapse.
	//
	// CAVEAT: these thresholds are configuration-specific. Deploying a flap (or
	// changing profile) alters camber, the pressure field and the surface itself,
	// and a deflected flap's own suction side separates as normal — so the same
	// fraction means something different. The percentage stays meaningful (it is
	// normalized), but the categorical verdict is approximate off the clean foil.
	sepOnset = 0.08 // trailing-edge separation starting to grow
	sepStall = 0.28 // large separation: flag the flow as stalled
)

// nBins is one Cl sample per integer degree across the sweep range.
const nBins = aoaMax - aoaMin + 1

// Window layout: the simulation viewport sits top-left, a data panel runs down
// the right side, and a controls panel spans the bottom, so text never overlaps
// the flow.
const (
	simW    = gridW * pixScale
	simH    = gridH * pixScale
	sidePnW = 300
	botPnH  = 156
	winW    = simW + sidePnW
	winH    = simH + botPnH
)

// UI palette.
var (
	colPanel  = color.RGBA{0x10, 0x12, 0x18, 0xff}
	colSep    = color.RGBA{0x2a, 0x30, 0x3c, 0xff}
	colHeader = color.RGBA{0x4c, 0xc6, 0xff, 0xff}
	colLabel  = color.RGBA{0x8a, 0x93, 0xa0, 0xff}
	colValue  = color.RGBA{0xee, 0xf2, 0xf8, 0xff}
	colBody   = color.RGBA{0x1a, 0x1d, 0x24, 0xff}

	colDrag   = color.RGBA{0xff, 0x90, 0x20, 0xff}
	colLift   = color.RGBA{0x40, 0xff, 0x60, 0xff}
	colRes    = color.RGBA{0xff, 0x40, 0xc0, 0xff}
	colCG     = color.RGBA{0xff, 0xe0, 0x40, 0xff}
	colPresHi = color.RGBA{0xff, 0x60, 0x40, 0xff} // pressure (push) arrows
	colPresLo = color.RGBA{0x50, 0x9c, 0xff, 0xff} // suction (pull) arrows

	colOK    = color.RGBA{0x6f, 0xe0, 0x8a, 0xff} // attached flow
	colWarn  = color.RGBA{0xff, 0xc6, 0x4c, 0xff} // separation growing
	colStall = color.RGBA{0xff, 0x5a, 0x5a, 0xff} // stalled
)

var uiFace = text.NewGoXFace(basicfont.Face7x13)

// fieldMode selects which scalar the background paints.
type fieldMode int

const (
	modeSpeed fieldMode = iota
	modeVorticity
	modePressure
	modeCount
)

var profiles = []string{"2412", "0012", "0009", "4412", "6412", "2415"}

// Game is the Ebiten model: a solver, the tracer cloud, the current geometry,
// and the layered images used to draw the field and the persistent smoke trail.
type Game struct {
	sim   *lbm.Solver
	smoke *viz.Particles

	profileIdx  int     // index into profiles for Tab-cycling presets
	nacaCode    string  // active NACA 4-digit code (any code, not just a preset)
	nacaInput   string  // NACA code being typed in the toolbar field
	alphaDeg    float64 // angle of attack in degrees
	u0          float64
	mode        fieldMode
	paused      bool
	streamlines bool // overlay integrated streamlines
	glow        bool // additive bloom on the smoke

	outline []foil.Point // chord-normalized profile, regenerated on profile change

	fieldImg *ebiten.Image // gridW×gridH scalar field
	trailImg *ebiten.Image // sim-sized accumulating smoke layer
	dotImg   *ebiten.Image // small tracer sprite
	fadeImg  *ebiten.Image // 1×1 translucent black for trail decay
	pixbuf   []byte        // reusable RGBA buffer for the field

	fxEMA, fyEMA, mzEMA float64 // smoothed forces, for steady arrows and CoP
	sepEMA              float64 // smoothed surface separation fraction (stall)
	clCur, cdCur        float64 // current smoothed coefficients

	// Live lift curve: one Cl sample per degree, filled in as the angle of
	// attack is swept. clSeen marks which bins have been visited.
	clCurve [nBins]float64
	clSeen  [nBins]bool

	sliders ui.Context // minigui context for the bottom panel's sliders

	bloomFx bloom

	// Scene mode: when scn is non-nil the body comes from a loaded, animated
	// scene instead of the interactive single foil. The foil controls (AoA,
	// profile) and the foil-specific markers are inert while it is active.
	scn         *scene.Scene
	scenePath   string // path of the loaded scene, for the panel
	savePath    string // file to write on plain Save (empty -> Save As prompts)
	animTime    float64
	animPlaying bool // timeline running; when false the surfaces hold their pose
	sceneErr    string
	simErr      string // status note after an instability reset; cleared on user changes

	// Editor mode: a second view of the same scene (toggled with E). The
	// simulation is frozen while editing.
	editing                bool
	cam                    camera
	selObj                 int
	dragStartX, dragStartY float64
	dragLastX, dragLastY   float64
	dragMoved              bool

	// Editor sub-mode: GEOMETRY edits the base shape/pivot; ANIMATE scrubs the
	// timeline and poses keyframes. editTime is the editor's scrub position
	// (independent of the simulator's animTime).
	editMode  editMode
	editTime  float64
	scrubbing bool // dragging the timeline playhead
	selKey    int  // index of the selected object's keyframe under the playhead, or -1

	// Keyframe pose clipboard (copy/paste across times and objects).
	poseClip    scene.Pose
	poseClipSet bool

	// Dragging a keyframe along the timeline to retime it (lifted out of Keys
	// during the drag, dropped back on release).
	draggingKey bool
	dragKeyT    float64
	dragKeyPose scene.Pose

	// Object clipboard (copy/paste/cut whole objects in geometry mode).
	objClip *scene.Object

	// Double-click detection (frame-based) and the pending endpoint when
	// reconnecting a broken outline.
	tick          uint64
	lastClickTick uint64
	lastClickX    float64
	lastClickY    float64
	connectFrom   int        // loose-end index chosen to reconnect, or -1
	snapOn        bool       // snap placed/dragged points to the grid and nearby vertices
	gui           ui.Context // minigui context for the toolbar (editor bottom / sim top)
	side          ui.Context // minigui context for the editor's object list + rename field

	// Native menu bar (glaze/menu). Menu clicks fire on the main thread; they
	// enqueue actions here to run on the game (Update) goroutine, avoiding races.
	// The menu is rebuilt only when menuSig (the context) changes.
	menuSig string
	quit    bool
	pendMu  sync.Mutex
	pending []func()

	// Pen draw tool: while drawing, each press places an anchor and the drag
	// until release becomes its Bezier tangent (click = corner). draftPts are the
	// anchors, draftH their tangents; penActive/penAnchor track the current press.
	drawing   bool
	draftPts  []foil.Point
	draftH    []foil.Point
	penActive bool
	penAnchor foil.Point

	// Transform gizmo state, captured at mouse-press and held for the drag.
	dragK          dragKind
	dragVtx        int          // vertex index being dragged (dragVertex/dragHandle)
	dragHandleSign float64      // +1 the out knob, -1 the in knob (dragHandle)
	dragOrig       []foil.Point // selected object's shape at press (geometry mode)
	dragOrigPivot  foil.Point   // selected object's pivot at press (geometry mode)
	dragPose0      scene.Pose   // selected object's pose at press (animate mode)
	dragWX0        float64      // world cursor x at press
	dragWY0        float64      // world cursor y at press
	dragA0         float64      // pivot->cursor angle at press (rotate)
	dragD0         float64      // pivot->cursor distance at press (scale)

	// Single-level undo for an edit (yagni: one level; upgrade to a stack if
	// editing gets heavy). Captures the whole object: geometry and keyframes.
	undoObj    int
	undoShape  []foil.Point
	undoHandle []foil.Point
	undoPivot  foil.Point
	undoGaps   []bool
	undoKeys   []scene.Key
	undoValid  bool
}

// NewGame builds the simulation, geometry and render targets.
func NewGame() *Game {
	g := &Game{
		alphaDeg:  4,
		u0:        defaultU,
		glow:      true,
		nacaCode:  profiles[0],
		nacaInput: profiles[0],
	}
	g.sim = lbm.New(gridW, gridH, tau, g.u0)
	g.smoke = viz.NewParticles(nParticles, gridW, gridH, 1)

	g.fieldImg = ebiten.NewImage(gridW, gridH)
	g.trailImg = ebiten.NewImage(simW, simH)
	g.dotImg = ebiten.NewImage(2, 2)
	g.dotImg.Fill(color.White)
	g.fadeImg = ebiten.NewImage(1, 1)
	g.fadeImg.Fill(color.RGBA{0, 0, 0, 26}) // controls smoke trail length
	g.pixbuf = make([]byte, gridW*gridH*4)

	st := ui.DefaultStyle()
	st.FieldW = 250 // slider track width in the bottom panel
	g.sliders.SetStyle(st)

	g.applyBody(true)
	return g
}

// applyBody rasterizes the current profile/AoA into the solver. reset wipes the
// flow (for a big change like a new profile); otherwise the mask is swapped in
// place so the flow keeps evolving smoothly.
func (g *Game) applyBody(reset bool) {
	out, err := foil.NACA(g.nacaCode, 80)
	if err != nil {
		out, _ = foil.NACA4("0012", 80) // stay safe on a bad code
	}
	g.outline = out
	mask := foil.Rasterize(g.placedOutline(), gridW, gridH)
	if reset {
		g.sim.SetSolid(mask)
		return
	}
	g.sim.UpdateSolid(mask)
}

// setNACA switches the interactive foil to a NACA 4-digit code (any valid code,
// not just a preset), rebuilding the body and clearing the stale lift curve. An
// invalid code is ignored and the input reverts to the current code.
func (g *Game) setNACA(code string) {
	// Easter egg: a neko (cat) is a (very draggy) "airfoil" too.
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "neko", "cat", "meow":
		g.setScene(nekoScene(), "neko (meow)")
		g.nacaInput = g.nacaCode // we left foil mode; restore the field text
		return
	}
	_, err := foil.NACA(code, 80)
	if err != nil {
		g.nacaInput = g.nacaCode
		return
	}
	g.nacaCode = code
	g.nacaInput = code
	g.simErr = ""
	g.applyBody(true)
	g.resetCurve()
}

// selectFoil returns to the interactive foil with the given NACA code, leaving
// any loaded scene (so the Foil menu works even from a scene like the neko).
func (g *Game) selectFoil(code string) {
	g.scn = nil
	g.scenePath = ""
	g.savePath = ""
	g.setNACA(code)
}

// pivot returns the grid-space pitch axis (quarter chord).
func (g *Game) pivot() (x, y float64) {
	chord := chordFrac * gridW
	return leadXFrac*gridW + pivotFrac*chord, float64(gridH) / 2
}

// placedOutline returns the profile in grid coordinates for the current AoA.
func (g *Game) placedOutline() []foil.Point {
	chord := chordFrac * gridW
	px, py := g.pivot()
	alpha := g.alphaDeg * math.Pi / 180
	return foil.Place(g.outline, chord, px, py, alpha, pivotFrac)
}

// sceneGlobal applies the angle of attack as a nose-up rotation of the whole
// aircraft about the pitch axis, on top of an object's own animated pose.
func (g *Game) sceneGlobal(poly []foil.Point) []foil.Point {
	if g.alphaDeg == 0 {
		return poly
	}
	px, py := g.pivot()
	return scene.Apply(poly, foil.Point{X: px, Y: py}, scene.Pose{Rot: -g.alphaDeg, Scale: 1})
}

// sceneMask rasterizes the union of all scene objects at time t, with the global
// angle of attack applied.
func (g *Game) sceneMask(t float64) []bool {
	mask := make([]bool, gridW*gridH)
	for _, o := range g.scn.Objects {
		if o.Broken() {
			continue // a cut outline is not a solid until it is closed
		}
		foil.RasterizeInto(mask, g.sceneGlobal(o.PolygonAt(t)), gridW, gridH)
	}
	return mask
}

// Update steps the simulation and handles input.
// enqueue schedules f to run on the game goroutine at the next Update. Menu
// callbacks (main thread) use it so they never touch game state concurrently.
func (g *Game) enqueue(f func()) {
	g.pendMu.Lock()
	g.pending = append(g.pending, f)
	g.pendMu.Unlock()
}

// drainPending runs and clears queued actions (on the game goroutine).
func (g *Game) drainPending() {
	g.pendMu.Lock()
	fns := g.pending
	g.pending = nil
	g.pendMu.Unlock()
	for _, f := range fns {
		f()
	}
}

// menuItems builds the native menu bar. The top-level menus are stable (App,
// File, Edit, View, Foil, Animate) so the bar never changes shape underfoot;
// items enable/disable and check-mark themselves per the current context. Only
// File uses Cmd shortcuts, so the rest never steal Cmd+C/V/Z from a focused text
// field. Menu clicks are enqueued onto the game goroutine.
func (g *Game) menuItems() []menu.Item {
	act := func(f func()) func() { return func() { g.enqueue(f) } }
	mark := func(on bool) string {
		if on {
			return "✔ "
		}
		return "    "
	}
	editToggle := "Edit Shape"
	if g.editing {
		editToggle = "Back to Simulator"
	}
	inGeom := g.editing && g.editMode == emGeometry
	inAnim := g.editing && g.editMode == emAnimate

	edit := []menu.Item{
		{Title: editToggle, OnClick: act(g.toggleEdit)},
		{Title: "Undo", Disabled: !g.editing, OnClick: act(g.undoEdit)},
		{Separator: true},
		{Title: mark(inGeom) + "Geometry mode", Disabled: !g.editing, OnClick: act(func() {
			if g.editMode != emGeometry {
				g.toggleEditMode()
			}
		})},
		{Title: mark(inAnim) + "Animate mode", Disabled: !g.editing, OnClick: act(func() {
			if g.editMode != emAnimate {
				g.toggleEditMode()
			}
		})},
		{Title: mark(g.snapOn) + "Snap", Disabled: !g.editing, OnClick: act(func() { g.snapOn = !g.snapOn })},
		{Separator: true},
		{Title: "Copy Object", Disabled: !inGeom || g.selObj < 0, OnClick: act(g.copyObject)},
		{Title: "Paste Object", Disabled: !inGeom || g.objClip == nil, OnClick: act(g.pasteObject)},
		{Title: "Cut Object", Disabled: !inGeom || g.selObj < 0, OnClick: act(g.cutObject)},
		{Title: "Merge with Clipboard", Disabled: !inGeom || g.selObj < 0 || g.objClip == nil, OnClick: act(g.mergeWithClipboard)},
	}

	foilItems := make([]menu.Item, 0, len(profiles)+2)
	for _, code := range profiles {
		c := code
		foilItems = append(foilItems, menu.Item{Title: mark(g.scn == nil && g.nacaCode == c) + "NACA " + c, OnClick: act(func() { g.selectFoil(c) })})
	}
	foilItems = append(foilItems,
		menu.Item{Separator: true},
		menu.Item{Title: "    Neko 🐱", OnClick: act(func() { g.setScene(nekoScene(), "neko (meow)") })},
	)

	animate := []menu.Item{
		{Title: "Set Keyframe", Disabled: !inAnim || g.selObj < 0, OnClick: act(g.setKeyframe)},
		{Title: "Delete Keyframe", Disabled: !inAnim || g.selObj < 0, OnClick: act(g.deleteKeyframe)},
		{Separator: true},
		{Title: "Copy Pose", Disabled: !inAnim || g.selObj < 0, OnClick: act(g.copyPose)},
		{Title: "Paste Pose", Disabled: !inAnim || g.selObj < 0 || !g.poseClipSet, OnClick: act(g.pastePose)},
		{Separator: true},
		{Title: "Loop -0.5s", Disabled: !inAnim, OnClick: act(func() { g.loopDelta(-0.5) })},
		{Title: "Loop +0.5s", Disabled: !inAnim, OnClick: act(func() { g.loopDelta(0.5) })},
	}

	return []menu.Item{
		{Title: "kutta", Submenu: []menu.Item{
			{Title: "Quit kutta", Shortcut: "cmd+q", OnClick: act(func() { g.quit = true })},
		}},
		{Title: "File", Submenu: []menu.Item{
			{Title: "Open…", Shortcut: "cmd+o", OnClick: act(g.openSceneDialog)},
			{Title: "Import SVG…", OnClick: act(g.importSVGDialog)},
			{Separator: true},
			{Title: "Save", Shortcut: "cmd+s", OnClick: act(g.saveScene)},
			{Title: "Save As…", Shortcut: "cmd+shift+s", OnClick: act(g.saveSceneAs)},
		}},
		{Title: "Edit", Submenu: edit},
		{Title: "View", Submenu: []menu.Item{
			{Title: mark(g.mode == modeSpeed) + "Speed", OnClick: act(func() { g.mode = modeSpeed })},
			{Title: mark(g.mode == modeVorticity) + "Vorticity", OnClick: act(func() { g.mode = modeVorticity })},
			{Title: mark(g.mode == modePressure) + "Pressure", OnClick: act(func() { g.mode = modePressure })},
			{Separator: true},
			{Title: mark(g.streamlines) + "Streamlines", OnClick: act(func() { g.streamlines = !g.streamlines })},
			{Title: mark(g.glow) + "Glow", OnClick: act(func() { g.glow = !g.glow })},
			{Title: mark(g.paused) + "Pause", OnClick: act(func() { g.paused = !g.paused })},
		}},
		{Title: "Foil", Submenu: foilItems},
		{Title: "Animate", Submenu: animate},
	}
}

// menuSignature captures the context the menu depends on; the menu is rebuilt
// only when it changes (not every frame).
func (g *Game) menuSignature() string {
	return fmt.Sprintf("%v|%v|%v|%v|%v|%v|%v|%v|%v|%s|%v|%v",
		g.editing, g.editMode, g.scn != nil, g.selObj >= 0, g.mode,
		g.streamlines, g.glow, g.paused, g.snapOn, g.nacaCode,
		g.objClip != nil, g.poseClipSet)
}

// syncMenu rebuilds the native menu on the main thread when the context changed.
func (g *Game) syncMenu() {
	sig := g.menuSignature()
	if sig == g.menuSig {
		return
	}
	items := g.menuItems()
	// Windows attaches the menu bar to the window handle; macOS ignores it.
	// Menu clicks are marshaled onto the game goroutine by act(), so no
	// Dispatch is needed on any platform.
	hwnd := mainWindowHandle()
	var err error
	ebiten.RunOnMainThread(func() {
		_, err = menu.Set(items, menu.Options{Window: hwnd})
	})
	if err != nil && hwnd == nil && runtime.GOOS == "windows" {
		return // the window is not up yet; retry next frame with a real handle
	}
	g.menuSig = sig // set, or unsupported here (Linux): either way, done
}

func (g *Game) Update() error {
	g.syncMenu()
	g.drainPending()
	g.handleDroppedFiles()
	if g.quit {
		return ebiten.Termination
	}
	// E toggles the editor, unless a text field is being typed into.
	if inpututil.IsKeyJustPressed(ebiten.KeyE) && !g.side.HasFocus() && !g.gui.HasFocus() {
		g.toggleEdit()
	}
	if g.editing {
		g.editorInput()
		return nil
	}
	g.handleInput()
	if g.paused {
		return nil
	}
	// In scene mode, advance the animation and swap the union mask in place so
	// the wake carries over (the validated UpdateSolid path).
	// Advance the timeline only while playing; the fluid keeps simulating either
	// way, so a frozen pose still develops its steady flow.
	if g.scn != nil && g.animPlaying {
		g.animTime += animDt
		g.sim.UpdateSolid(g.sceneMask(g.scn.LoopTime(g.animTime)))
	}
	g.stepSim(substeps)
	g.smoke.Step(g.sim, tracerSpeed)
	const a = 0.04 // EMA smoothing for the displayed forces
	g.fxEMA += a * (g.sim.Fx - g.fxEMA)
	g.fyEMA += a * (g.sim.Fy - g.fyEMA)
	g.mzEMA += a * (g.sim.Mz - g.mzEMA)
	g.sepEMA += a * (g.sim.Sep - g.sepEMA)

	denom := 0.5 * g.u0 * g.u0 * chordFrac * gridW
	if denom > 0 {
		g.clCur = g.fyEMA / denom
		g.cdCur = g.fxEMA / denom
	}
	// Record lift into the per-degree bin for the current angle (both modes; in
	// scene mode the flap phase adds some scatter). Sweep the angle to trace it.
	bin := int(math.Round(g.alphaDeg)) - aoaMin
	if bin >= 0 && bin < nBins {
		g.clCurve[bin] = g.clCur
		g.clSeen[bin] = true
	}
	return nil
}

// stepSim advances the solver n steps and verifies the state stayed finite.
// The collide clamp in lbm should make a blow-up impossible, but if some
// unforeseen corner still produces NaN this backstop resets the flow in place
// instead of letting a sick field reach the renderer (issue #1: the colormap
// used to panic on it). Both stepping sites — the frame loop and the N
// single-step — must go through here.
func (g *Game) stepSim(n int) {
	for range n {
		g.sim.Step()
	}
	if g.sim.Finite() {
		return
	}
	g.resetUnstableFlow()
}

// resetUnstableFlow rebuilds the flow from clean inflow around the CURRENT
// body — the scene mask when a scene is loaded, the interactive foil otherwise
// — and clears every derived readout the blow-up polluted.
func (g *Game) resetUnstableFlow() {
	// Re-sync the inlet speed from the app's state first: whatever poisoned the
	// solver must not survive into the rebuilt flow.
	g.sim.SetInletSpeed(g.u0)
	if g.scn != nil {
		g.sim.SetSolid(g.sceneMask(g.scn.LoopTime(g.animTime)))
	} else {
		g.applyBody(true)
	}
	g.smoke = viz.NewParticles(nParticles, gridW, gridH, 1)
	g.fxEMA, g.fyEMA, g.mzEMA, g.sepEMA = 0, 0, 0, 0
	g.clCur, g.cdCur = 0, 0
	g.simErr = "flow reset after numerical instability"
}

// resetCurve clears the lift curve; the polar is specific to one profile.
func (g *Game) resetCurve() {
	g.clCurve = [nBins]float64{}
	g.clSeen = [nBins]bool{}
}

// fieldName is the label for the current scalar field.
func fieldName(m fieldMode) string {
	switch m {
	case modeVorticity:
		return "Vorticity"
	case modePressure:
		return "Pressure"
	default:
		return "Speed"
	}
}

// runSimToolbar drives the simulator's clickable toolbar (minigui) over the
// flow's top-left: edit, file actions, the field-mode cycle and pause. The
// hotkeys keep working alongside it.
func (g *Game) runSimToolbar() {
	g.gui.Begin(ui.InputFromEbiten(), 8, 8)
	if g.gui.Button("st.edit", "Edit") {
		g.toggleEdit()
	}
	g.gui.SameLine()
	if g.gui.Button("st.open", "Open") {
		g.openSceneDialog()
	}
	g.gui.SameLine()
	if g.gui.Button("st.save", "Save") {
		g.saveScene()
	}
	g.gui.SameLine()
	if g.gui.Button("st.saveas", "Save As") {
		g.saveSceneAs()
	}
	g.gui.SameLine()
	if g.gui.Button("st.field", fieldName(g.mode)) {
		g.mode = (g.mode + 1) % modeCount
	}
	g.gui.SameLine()
	if g.gui.Toggle("st.pause", "Pause", g.paused) {
		g.paused = !g.paused
	}
	// NACA code entry (interactive foil only): type a 4- or 5-digit code, Enter applies.
	if g.scn == nil {
		g.gui.SameLine()
		g.gui.Label("NACA")
		g.gui.SameLine()
		g.gui.SetItemWidth(60) // fits a 5-digit code
		g.gui.TextField("st.naca", &g.nacaInput)
		if g.gui.Submitted("st.naca") {
			g.setNACA(g.nacaInput)
		}
	}
	g.gui.End()
}

func (g *Game) handleInput() {
	g.runSimToolbar() // immediate-mode: build + handle the toolbar every frame
	// While typing in the NACA field, let it own the keyboard (sliders still work).
	if g.gui.HasFocus() {
		g.runSliders()
		return
	}
	// L plays/pauses the timeline of an open scene. The fluid keeps simulating
	// either way, so the surfaces can be frozen at any pose while the flow
	// settles. It does nothing with no scene open (open one via O / the menu).
	if inpututil.IsKeyJustPressed(ebiten.KeyL) && g.scn != nil {
		g.animPlaying = !g.animPlaying
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) && g.scn != nil {
		g.scn = nil
		g.animPlaying = false
		g.applyBody(true)
	}
	// O opens a scene file through the native dialog.
	if inpututil.IsKeyJustPressed(ebiten.KeyO) {
		g.openSceneDialog()
	}
	// Angle of attack works in both modes: it pitches the foil, or the whole
	// aircraft in scene mode.
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.setAlpha(g.alphaDeg + 1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.setAlpha(g.alphaDeg - 1)
	}
	// Cycling the NACA profile only applies to the interactive foil.
	if g.scn == nil && inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		g.profileIdx = (g.profileIdx + 1) % len(profiles)
		g.setNACA(profiles[g.profileIdx])
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyV) {
		g.mode = (g.mode + 1) % modeCount
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		switch {
		case ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight):
			if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
				g.saveSceneAs() // Cmd+Shift+S
			} else {
				g.saveScene() // Cmd+S: write to the current file
			}
		default:
			g.streamlines = !g.streamlines
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyG) {
		g.glow = !g.glow
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.paused = !g.paused
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyN) && g.paused {
		g.stepSim(1)
		g.smoke.Step(g.sim, tracerSpeed)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.simErr = ""
		g.sim.Reset()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) {
		g.setSpeed(g.u0 + 0.01)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) {
		g.setSpeed(g.u0 - 0.01)
	}
	g.runSliders()
}

// runSliders drives the bottom panel's draggable sliders (minigui): angle of
// attack and inlet speed, each with a label row whose value column stays put
// while the knob moves.
func (g *Game) runSliders() {
	g.sliders.Begin(ui.InputFromEbiten(), simW+16, simH+30)
	g.sliders.Label(fmt.Sprintf("Angle of attack %17.1f deg", g.alphaDeg))
	if g.sliders.Slider("aoa", &g.alphaDeg, -aoaLimit, aoaLimit) {
		g.setAlpha(g.alphaDeg)
	}
	g.sliders.Label(fmt.Sprintf("Inlet speed %25.2f", g.u0))
	if g.sliders.Slider("spd", &g.u0, spdMin, spdMax) {
		g.setSpeed(g.u0)
	}
	g.sliders.End()
}

// openSceneDialog asks the OS for a scene file and loads it (paused at the
// start). A cancel or a load error leaves the current state unchanged.
func (g *Game) openSceneDialog() {
	// The native panel is AppKit, which must run on the main thread; Update runs
	// on a worker goroutine in Ebiten's default multithreaded mode, so hop over
	// with RunOnMainThread (it blocks until the panel closes).
	var path string
	ebiten.RunOnMainThread(func() {
		path = filedialog.Open(filedialog.Options{
			Title:      "Open airfoil scene (" + sceneio.Ext + ")",
			Extensions: []string{sceneio.Ext[1:]},
		})
	})
	if path == "" {
		g.noDialogHint()
		return // cancelled, or unsupported platform
	}
	src, err := os.ReadFile(path) // #nosec G304 -- path chosen by the user via the native dialog
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	sc, err := sceneio.Load(string(src))
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	g.setScene(sc, path)
	g.savePath = path // the file just opened is the natural target for plain Save
}

// setScene switches to a loaded scene, paused at t=0, and pushes its solid to the
// solver. It clears the save target (callers that opened a file set it after).
func (g *Game) setScene(sc *scene.Scene, path string) {
	g.scn = sc
	g.scenePath = path
	g.savePath = ""
	g.animTime = 0
	g.animPlaying = false
	g.sceneErr = ""
	g.simErr = ""
	g.resetCurve()
	g.sim.SetSolid(g.sceneMask(0))
}

// sceneToSave is the scene to serialize: the loaded scene, or the interactive
// foil wrapped as a single static object.
func (g *Game) sceneToSave() *scene.Scene {
	if g.scn != nil {
		return g.scn
	}
	return &scene.Scene{Objects: []*scene.Object{{Name: "airfoil", Shape: g.placedOutline()}}}
}

// saveScene writes to the current file without prompting; with no current file
// (nothing saved/opened yet) it falls back to Save As.
func (g *Game) saveScene() {
	if g.savePath == "" {
		g.saveSceneAs()
		return
	}
	text, err := sceneio.Save(g.sceneToSave())
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	err = os.WriteFile(g.savePath, []byte(text), 0o600) // #nosec G304 -- previously user-chosen path
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	g.sceneErr = ""
}

// saveSceneAs always prompts for a file, prefilling the current name.
func (g *Game) saveSceneAs() {
	text, err := sceneio.Save(g.sceneToSave())
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	name := "untitled" + sceneio.Ext
	if g.savePath != "" {
		name = filepath.Base(g.savePath)
	}
	var path string
	ebiten.RunOnMainThread(func() {
		path = filedialog.Save(filedialog.Options{
			Title:    "Save airfoil scene (" + sceneio.Ext + ")",
			Filename: name,
		})
	})
	if path == "" {
		return // cancelled
	}
	if !strings.HasSuffix(path, sceneio.Ext) {
		path += sceneio.Ext
	}
	err = os.WriteFile(path, []byte(text), 0o600) // #nosec G304 -- user-chosen save path
	if err != nil {
		g.sceneErr = err.Error()
		return
	}
	g.scenePath = path
	g.savePath = path
}

// setAlpha changes the angle of attack. In foil mode it re-rasterizes the body
// (without resetting the flow); in scene mode the angle is applied as a global
// rotation each frame, so nothing else is needed here.
func (g *Game) setAlpha(deg float64) {
	g.simErr = ""
	g.alphaDeg = math.Max(-aoaLimit, math.Min(aoaLimit, deg))
	if g.scn == nil {
		g.applyBody(false)
		return
	}
	// Scene mode: re-apply immediately so the aircraft pitches even when the
	// timeline is paused.
	g.sim.UpdateSolid(g.sceneMask(g.scn.LoopTime(g.animTime)))
}

// setSpeed changes the free-stream speed in place (no reset).
func (g *Game) setSpeed(u float64) {
	g.simErr = ""
	g.u0 = math.Max(0.02, math.Min(0.15, u))
	g.sim.SetInletSpeed(g.u0)
}

// Draw paints the clipped simulation viewport and the two information panels —
// or the editor, when in edit mode.
func (g *Game) Draw(screen *ebiten.Image) {
	if g.editing {
		g.drawEditor(screen)
		return
	}
	screen.Fill(colPanel)

	// A sub-image clips every flow overlay (field, smoke, arrows) to the
	// viewport so nothing bleeds onto the panels.
	vp := screen.SubImage(image.Rect(0, 0, simW, simH)).(*ebiten.Image)

	g.paintField()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(pixScale, pixScale)
	op.Filter = ebiten.FilterLinear
	vp.DrawImage(g.fieldImg, op)

	g.drawSmoke(vp)
	if g.streamlines {
		g.drawStreamlines(vp)
	}
	g.drawOutline(vp)
	if g.mode == modePressure {
		g.drawSurfacePressure(vp)
	}
	g.drawMarkers(vp)
	g.drawForces(vp)

	vector.StrokeRect(screen, 0, 0, simW, simH, 1, colSep, false)
	g.drawSidePanel(screen)
	g.drawBottomPanel(screen)
	g.gui.Render(screen)     // the minigui toolbar, over the flow's top-left
	g.sliders.Render(screen) // the bottom panel's sliders
}

// paintField fills the reusable pixel buffer from the chosen scalar field. Grid
// row 0 is the bottom in physics coordinates, so it maps to the bottom image row.
func (g *Game) paintField() {
	for y := range gridH {
		row := gridH - 1 - y
		for x := range gridW {
			c := g.fieldColor(x, y)
			idx := (row*gridW + x) * 4
			g.pixbuf[idx] = c.R
			g.pixbuf[idx+1] = c.G
			g.pixbuf[idx+2] = c.B
			g.pixbuf[idx+3] = 0xff
		}
	}
	g.fieldImg.WritePixels(g.pixbuf)
}

func (g *Game) fieldColor(x, y int) color.RGBA {
	if g.sim.Solid(x, y) {
		return colBody
	}
	c := y*gridW + x
	switch g.mode {
	case modeVorticity:
		return viz.Vorticity(g.sim.Vorticity(x, y), vortScale)
	case modePressure:
		return viz.Pressure(g.cp(c), cpScale)
	default:
		speed := math.Hypot(g.sim.Ux[c], g.sim.Uy[c])
		return viz.Speed(speed / (g.u0 * 2))
	}
}

// cp is the pressure coefficient at cell c, from the lattice equation of state
// p = rho/3 referenced to the free stream (rho0 = 1).
func (g *Game) cp(c int) float64 {
	return (g.sim.Rho[c] - 1) / (1.5 * g.u0 * g.u0)
}

// drawSmoke fades the trail layer, stamps each tracer onto it, then adds the
// whole layer over the field so streaklines glow like real smoke.
func (g *Game) drawSmoke(dst *ebiten.Image) {
	fade := &ebiten.DrawImageOptions{}
	fade.GeoM.Scale(float64(simW), float64(simH))
	g.trailImg.DrawImage(g.fadeImg, fade)

	// Tint each tracer by the local flow speed (blue slow -> red fast), so the
	// smoke reads as a speed field even over the vorticity/pressure background.
	inv := 0.0
	if g.u0 > 0 {
		inv = 1 / (2 * g.u0)
	}
	for i := range g.smoke.X {
		x, y := g.smoke.X[i], g.smoke.Y[i]
		ux, uy := g.sim.VelocityAt(x, y)
		col := viz.Speed(math.Min(1, math.Hypot(ux, uy)*inv))
		sx := x * pixScale
		sy := (float64(gridH-1) - y) * pixScale
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(sx, sy)
		op.ColorScale.ScaleWithColor(col)
		op.ColorScale.ScaleAlpha(0.85)
		g.trailImg.DrawImage(g.dotImg, op)
	}

	add := &ebiten.DrawImageOptions{}
	add.Blend = ebiten.BlendLighter
	dst.DrawImage(g.trailImg, add)

	// Bloom: a soft additive halo around the bright smoke streaks.
	if g.glow {
		g.bloomFx.apply(dst, g.trailImg, 2.5, 2, 0.9)
	}
}

// drawStreamlines overlays instantaneous streamlines, integrated from a column
// of seeds near the inlet by stepping a fixed arc length along the local
// velocity (RK2 midpoint). The whole set is one batched path, so it is a single
// draw call regardless of length.
func (g *Game) drawStreamlines(dst *ebiten.Image) {
	const nLines = 28
	const maxSteps = 400
	const ds = 1.5 // grid cells advanced per step
	var path vector.Path
	for k := range nLines {
		y := (float64(k) + 0.5) / nLines * float64(gridH)
		x := 2.0
		if g.solidAtGrid(x, y) {
			continue
		}
		sx, sy := gridToScreen(x, y)
		path.MoveTo(sx, sy)
		for range maxSteps {
			ux, uy := g.sim.VelocityAt(x, y)
			sp := math.Hypot(ux, uy)
			if sp < 1e-7 {
				break
			}
			mx := x + ux/sp*ds*0.5
			my := y + uy/sp*ds*0.5
			mux, muy := g.sim.VelocityAt(mx, my)
			msp := math.Hypot(mux, muy)
			if msp < 1e-7 {
				break
			}
			x += mux / msp * ds
			y += muy / msp * ds
			if x < 0 || x >= gridW-1 || y < 1 || y >= gridH-1 || g.solidAtGrid(x, y) {
				break
			}
			lx, ly := gridToScreen(x, y)
			path.LineTo(lx, ly)
		}
	}
	op := &vector.StrokeOptions{Width: 1, LineJoin: vector.LineJoinRound}
	dop := &vector.DrawPathOptions{AntiAlias: true}
	dop.ColorScale.ScaleWithColor(color.RGBA{0xde, 0xe8, 0xff, 0xc0})
	vector.StrokePath(dst, &path, op, dop)
}

// gridToScreen maps a grid-space point (y up) to viewport pixels (y down).
func gridToScreen(x, y float64) (float32, float32) {
	return float32(x * pixScale), float32((float64(gridH-1) - y) * pixScale)
}

// gridToScreenF is gridToScreen with float64 results, for the arrow helpers.
func gridToScreenF(x, y float64) (float64, float64) {
	return x * pixScale, (float64(gridH-1) - y) * pixScale
}

// drawOutline strokes the body edge(s) crisply on top of the blocky raster mask:
// each scene object in scene mode, otherwise the single foil.
func (g *Game) drawOutline(dst *ebiten.Image) {
	col := color.RGBA{0xff, 0xff, 0xff, 0xd0}
	if g.scn != nil {
		t := g.scn.LoopTime(g.animTime)
		for _, o := range g.scn.Objects {
			strokeClosed(dst, g.sceneGlobal(o.PolygonAt(t)), col)
		}
		return
	}
	strokeClosed(dst, g.placedOutline(), col)
}

// strokeClosed outlines a closed polygon in viewport space.
func strokeClosed(dst *ebiten.Image, poly []foil.Point, col color.Color) {
	for i := range poly {
		j := (i + 1) % len(poly)
		x0, y0 := gridToScreen(poly[i].X, poly[i].Y)
		x1, y1 := gridToScreen(poly[j].X, poly[j].Y)
		vector.StrokeLine(dst, x0, y0, x1, y1, 1.5, col, true)
	}
}

// drawSurfacePressure draws a little arrow along each stretch of the body
// surface: pointing inward where the flow presses (Cp > 0, warm) and outward
// where it sucks (Cp < 0, cool), with length proportional to |Cp|. The pressure
// is sampled from the fluid cell just off the surface.
func (g *Game) drawSurfacePressure(dst *ebiten.Image) {
	if g.scn != nil {
		return // foil-specific; the scene body uses a different outline
	}
	poly := g.placedOutline()
	const step = 3
	for i := 0; i < len(poly); i += step {
		j := (i + 1) % len(poly)
		mx := (poly[i].X + poly[j].X) / 2
		my := (poly[i].Y + poly[j].Y) / 2
		dx := poly[j].X - poly[i].X
		dy := poly[j].Y - poly[i].Y
		L := math.Hypot(dx, dy)
		if L < 1e-6 {
			continue
		}
		nx, ny := dy/L, -dx/L // edge normal; orient it toward the fluid below
		if g.solidAtGrid(mx+nx*1.5, my+ny*1.5) {
			nx, ny = -nx, -ny
		}
		rho, ok := g.rhoOutside(mx+nx*2, my+ny*2)
		if !ok {
			continue
		}
		cp := (rho - 1) / (1.5 * g.u0 * g.u0)
		length := math.Abs(cp) * cpVecK
		bx, by := gridToScreenF(mx, my)
		tx, ty := gridToScreenF(mx+nx*length, my+ny*length)
		if cp >= 0 {
			// pressure: tail outside the surface, head on it
			drawArrow(dst, tx, ty, bx, by, colPresHi)
			continue
		}
		// suction: tail on the surface, head pulling outward
		drawArrow(dst, bx, by, tx, ty, colPresLo)
	}
}

// solidAtGrid reports whether the grid cell containing (x,y) is body.
func (g *Game) solidAtGrid(x, y float64) bool {
	xi, yi := int(x), int(y)
	if xi < 0 || xi >= gridW || yi < 0 || yi >= gridH {
		return false
	}
	return g.sim.Solid(xi, yi)
}

// rhoOutside samples the density at (x,y) if it is a fluid cell in the domain.
func (g *Game) rhoOutside(x, y float64) (float64, bool) {
	xi, yi := int(x), int(y)
	if xi < 0 || xi >= gridW || yi < 0 || yi >= gridH {
		return 0, false
	}
	if g.sim.Solid(xi, yi) {
		return 0, false
	}
	return g.sim.Rho[yi*gridW+xi], true
}

// centerOfPressure intersects the (smoothed) resultant force's line of action
// with the chord line. ok is false near zero lift, where the point runs off to
// infinity and is not meaningful.
func (g *Game) centerOfPressure() (x, y, frac float64, ok bool) {
	if g.scn != nil {
		return 0, 0, 0, false // foil-specific; undefined for a multi-object scene
	}
	chord := chordFrac * gridW
	alpha := g.alphaDeg * math.Pi / 180
	sin, cos := math.Sincos(alpha)
	cx, cy := cos, -sin // chord unit direction in grid space
	px, py := g.pivot()
	lex := px - pivotFrac*chord*cx
	ley := py - pivotFrac*chord*cy
	denom := cx*g.fyEMA - cy*g.fxEMA
	if math.Abs(denom) < 1e-6 {
		return 0, 0, 0, false
	}
	t := (g.mzEMA - (lex*g.fyEMA - ley*g.fxEMA)) / denom
	frac = t / chord
	if frac < -0.1 || frac > 1.1 {
		return 0, 0, 0, false
	}
	return lex + t*cx, ley + t*cy, frac, true
}

// drawMarkers shows the suggested center of gravity (the aerodynamic center, a
// stable reference for trimming a model) and, when defined, the live center of
// pressure.
func (g *Game) drawMarkers(dst *ebiten.Image) {
	if g.scn != nil {
		return // the CG/CoP markers are foil-specific
	}
	px, py := g.pivot()
	cgx, cgy := gridToScreen(px, py)
	drawCGSymbol(dst, cgx, cgy, 7)

	cx, cy, _, ok := g.centerOfPressure()
	if ok {
		sx, sy := gridToScreen(cx, cy)
		vector.FillCircle(dst, sx, sy, 4, colRes, true)
	}
}

// drawCGSymbol draws the standard quartered center-of-gravity roundel.
func drawCGSymbol(dst *ebiten.Image, cx, cy, r float32) {
	vector.FillCircle(dst, cx, cy, r, color.RGBA{0x20, 0x20, 0x20, 0xff}, true)
	vector.StrokeLine(dst, cx-r, cy, cx+r, cy, 1, colCG, true)
	vector.StrokeLine(dst, cx, cy-r, cx, cy+r, 1, colCG, true)
	vector.StrokeRect(dst, cx-r, cy-r, 2*r, 2*r, 1, colCG, true)
}

// drawForces draws the lift (green), drag (orange) and resultant (magenta)
// vectors from the center of pressure, falling back to the pitch axis near zero
// lift where the center of pressure is undefined.
func (g *Game) drawForces(dst *ebiten.Image) {
	ax, ay, _, ok := g.centerOfPressure()
	if !ok {
		ax, ay = g.pivot()
		if g.scn != nil {
			ax, ay = g.sceneBodyCenter() // follow the body when it is moved/animated
		}
	}
	sx, sy := gridToScreen(ax, ay)
	ox, oy := float64(sx), float64(sy)
	lift := g.fyEMA * forceVisK
	drag := g.fxEMA * forceVisK
	drawArrow(dst, ox, oy, ox+drag, oy, colDrag)
	drawArrow(dst, ox, oy, ox, oy-lift, colLift)
	drawArrow(dst, ox, oy, ox+drag, oy-lift, colRes)
}

// sceneBodyCenter is the bounding-box center of the scene body as currently
// drawn (all objects at the animation time, with the global angle of attack), so
// the force vectors anchor on the body wherever it is moved or animated.
func (g *Game) sceneBodyCenter() (float64, float64) {
	t := g.scn.LoopTime(g.animTime)
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, o := range g.scn.Objects {
		for _, p := range g.sceneGlobal(o.PolygonAt(t)) {
			minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
			maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
		}
	}
	if math.IsInf(minX, 1) {
		return g.pivot()
	}
	return (minX + maxX) / 2, (minY + maxY) / 2
}

// drawArrow strokes a line with a small two-stroke arrowhead at its tip.
func drawArrow(dst *ebiten.Image, x0, y0, x1, y1 float64, clr color.Color) {
	vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), 2, clr, true)
	dx := x1 - x0
	dy := y1 - y0
	length := math.Hypot(dx, dy)
	if length < 6 {
		return
	}
	ux := dx / length
	uy := dy / length
	const h = 7
	// Two barbs angled back from the tip.
	lx := x1 - h*(ux*math.Cos(0.5)-uy*math.Sin(0.5))
	ly := y1 - h*(uy*math.Cos(0.5)+ux*math.Sin(0.5))
	rx := x1 - h*(ux*math.Cos(0.5)+uy*math.Sin(0.5))
	ry := y1 - h*(uy*math.Cos(0.5)-ux*math.Sin(0.5))
	vector.StrokeLine(dst, float32(x1), float32(y1), float32(lx), float32(ly), 2, clr, true)
	vector.StrokeLine(dst, float32(x1), float32(y1), float32(rx), float32(ry), 2, clr, true)
}

// drawSidePanel lists the live simulation data down the right edge.
func (g *Game) drawSidePanel(screen *ebiten.Image) {
	x := float64(simW + 16)
	y := 14.0

	nu := (tau - 0.5) / 3
	re := g.u0 * chordFrac * gridW / nu
	cl, cd := g.clCur, g.cdCur
	ld := 0.0
	if cd != 0 {
		ld = cl / cd
	}
	if g.scn != nil {
		state := "paused (L to play)"
		if g.animPlaying {
			state = "playing"
		}
		y = g.header(screen, "SCENE", x, y)
		y = g.row(screen, "Source", filepath.Base(g.scenePath), x, y)
		y = g.row(screen, "Objects", fmt.Sprintf("%d", len(g.scn.Objects)), x, y)
		y = g.row(screen, "Angle of attack", fmt.Sprintf("%+.1f deg", g.alphaDeg), x, y)
		y = g.row(screen, "Loop", fmt.Sprintf("%.1f s", g.scn.Loop), x, y)
		y = g.row(screen, "Time", fmt.Sprintf("%.1f s  [%s]", g.scn.LoopTime(g.animTime), state), x, y)
	} else {
		y = g.header(screen, "SIMULATION", x, y)
		y = g.row(screen, "Profile", "NACA "+g.nacaCode, x, y)
		y = g.row(screen, "Angle of attack", fmt.Sprintf("%+.1f deg", g.alphaDeg), x, y)
	}
	// Airspeed shown as a Mach number (Ma = u/cs, cs = 1/sqrt(3) in lattice units)
	// and a percent of the stable range, friendlier than the raw lattice speed;
	// the solver is near-incompressible, so this stays well under Mach 1.
	mach := g.u0 * math.Sqrt(3)
	y = g.row(screen, "Airspeed", fmt.Sprintf("Ma %.2f  (%.0f%%)", mach, 100*g.u0/spdMax), x, y)
	y = g.row(screen, "Reynolds", fmt.Sprintf("~ %.0f", re), x, y)

	y = g.header(screen, "FORCES (qualitative)", x, y+8)
	y = g.row(screen, "Lift  Cl", fmt.Sprintf("%+.2f", cl), x, y)
	y = g.row(screen, "Drag  Cd", fmt.Sprintf("%+.3f", cd), x, y)
	y = g.row(screen, "L / D", fmt.Sprintf("%+.1f", ld), x, y)
	stallStr, stallCol := stallStatus(g.sepEMA)
	y = g.rowc(screen, "Flow", stallStr, x, y, stallCol)
	if g.scn == nil {
		copStr := "n/a (near zero lift)"
		_, _, frac, ok := g.centerOfPressure()
		if ok {
			copStr = fmt.Sprintf("%.0f%% chord", frac*100)
		}
		y = g.row(screen, "Center of press.", copStr, x, y)
		y = g.row(screen, "CG (aero center)", fmt.Sprintf("%.0f%% chord", pivotFrac*100), x, y)
	}

	if g.simErr != "" {
		y = g.row(screen, "Status", g.simErr, x, y)
	}
	if g.sceneErr != "" {
		y = g.row(screen, "Scene error", g.sceneErr, x, y)
	}

	y = g.header(screen, "DISPLAY", x, y+8)
	// The colorbar carries its own field-name label above it, so drop it a little
	// further to clear the DISPLAY header.
	g.drawColorbar(screen, x, y+14, 200, 14)
}

// drawColorbar shows the color scale for the active field so colors read as
// values. For speed it spans 0..2*U; for vorticity it spans the diverging range.
func (g *Game) drawColorbar(screen *ebiten.Image, x, y, w, h float64) {
	const segs = 64
	for i := range segs {
		t := float64(i) / (segs - 1)
		var c color.RGBA
		switch g.mode {
		case modeVorticity:
			c = viz.Vorticity((t*2-1)*vortScale, vortScale)
		case modePressure:
			c = viz.Pressure((t*2-1)*cpScale, cpScale)
		default:
			c = viz.Speed(t)
		}
		vector.FillRect(screen, float32(x+t*w), float32(y), float32(w/segs+1), float32(h), c, false)
	}
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, colSep, false)
	switch g.mode {
	case modeVorticity:
		drawString(screen, "vorticity", x, y-16, colLabel)
		drawString(screen, "CW", x, y+h+4, colLabel)
		drawString(screen, "CCW", x+w-22, y+h+4, colLabel)
	case modePressure:
		drawString(screen, "pressure (Cp)", x, y-16, colLabel)
		drawString(screen, "suction", x, y+h+4, colLabel)
		drawString(screen, "high", x+w-26, y+h+4, colLabel)
	default:
		drawString(screen, "flow speed", x, y-16, colLabel)
		drawString(screen, "0", x, y+h+4, colLabel)
		drawString(screen, fmt.Sprintf("%.2f", g.u0*2), x+w-26, y+h+4, colLabel)
	}
}

// drawBottomPanel lists the keyboard controls and the draggable sliders.
func (g *Game) drawBottomPanel(screen *ebiten.Image) {
	top := float64(simH)
	vector.StrokeLine(screen, 0, float32(top), winW, float32(top), 1, colSep, false)

	x := 16.0
	y := g.header(screen, "CONTROLS", x, top+14)
	controls := [][2]string{
		{"Up / Down", "angle of attack"},
		{"Tab", "cycle profile"},
		{"V", "speed/vort/press"},
		{"S", "streamlines"},
		{"G", "glow / bloom"},
		{"[  ]", "inlet speed"},
		{"Space", "pause / resume"},
		{"N", "step (paused)"},
		{"R", "reset the flow"},
		{"L", "play/pause timeline"},
		{"O", "open .afoil file"},
		{"Cmd+S", "save scene"},
		{"E", "edit mode"},
		{"Esc", "back to foil"},
	}
	const colW = 250.0
	const rows = 7
	for i, c := range controls {
		cx := x + float64(i/rows)*colW
		cy := y + float64(i%rows)*16
		drawString(screen, c[0], cx, cy, colValue)
		drawString(screen, c[1], cx+86, cy, colLabel)
	}

	g.drawClPlot(screen, 540, top+24, 480, 116)
}

// drawClPlot draws the live lift curve (Cl vs angle of attack) collected as the
// angle is swept, with the current operating point highlighted.
func (g *Game) drawClPlot(screen *ebiten.Image, x, y, w, h float64) {
	drawString(screen, "LIFT CURVE  Cl vs AoA", x, y-16, colHeader)
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, colSep, false)

	// Map data coordinates to pixels.
	px := func(a float64) float64 { return x + (a-aoaMin)/(aoaMax-aoaMin)*w }
	py := func(cl float64) float64 {
		t := (cl - clPlotMin) / (clPlotMax - clPlotMin)
		return y + h - t*h
	}
	// Zero axes.
	zeroY := py(0)
	vector.StrokeLine(screen, float32(x), float32(zeroY), float32(x+w), float32(zeroY), 1, colSep, true)
	zeroX := px(0)
	vector.StrokeLine(screen, float32(zeroX), float32(y), float32(zeroX), float32(y+h), 1, colSep, true)
	drawString(screen, fmt.Sprintf("%d", aoaMin), x+2, y+h-14, colLabel)
	drawString(screen, fmt.Sprintf("+%d deg", aoaMax), x+w-52, y+h-14, colLabel)
	drawString(screen, fmt.Sprintf("%+g", clPlotMax), x+4, y+2, colLabel)

	// Connect the visited bins in order.
	prevX, prevY := 0.0, 0.0
	have := false
	seen := 0
	for b := range nBins {
		if !g.clSeen[b] {
			have = false
			continue
		}
		seen++
		ax := float64(b + aoaMin)
		cl := math.Max(clPlotMin, math.Min(clPlotMax, g.clCurve[b]))
		cxp, cyp := px(ax), py(cl)
		if have {
			vector.StrokeLine(screen, float32(prevX), float32(prevY), float32(cxp), float32(cyp), 1.5, colLift, true)
		}
		prevX, prevY = cxp, cyp
		have = true
	}
	if seen < 2 {
		drawString(screen, "sweep AoA (up/down) to trace the curve", x+20, y+h/2-6, colLabel)
	}

	// Current operating point, when it is within the plotted AoA range; past it
	// (e.g. broadside at 90 deg) the operating point is simply off-chart.
	if g.alphaDeg >= aoaMin && g.alphaDeg <= aoaMax {
		cl := math.Max(clPlotMin, math.Min(clPlotMax, g.clCur))
		vector.FillCircle(screen, float32(px(g.alphaDeg)), float32(py(cl)), 3, colValue, true)
	}
}

// header draws a section title and returns the y for the next line.
func (g *Game) header(screen *ebiten.Image, title string, x, y float64) float64 {
	drawString(screen, title, x, y, colHeader)
	return y + 22
}

// row draws a label/value pair and returns the y for the next line.
func (g *Game) row(screen *ebiten.Image, label, value string, x, y float64) float64 {
	return g.rowc(screen, label, value, x, y, colValue)
}

// rowc is row with an explicit value color, for status fields like stall.
func (g *Game) rowc(screen *ebiten.Image, label, value string, x, y float64, clr color.Color) float64 {
	drawString(screen, label, x, y, colLabel)
	drawString(screen, value, x+140, y, clr)
	return y + 18
}

// stallStatus maps the smoothed separation fraction to a label and color. The
// percentage is the reliable, shape-robust signal (a normalized fraction); the
// categorical verdict is a guide calibrated on the clean default foil and shifts
// with configuration (flap deflection, profile), so it is shown approximate.
func stallStatus(sep float64) (string, color.Color) {
	pct := sep * 100
	switch {
	case sep >= sepStall:
		return fmt.Sprintf("STALL ~ %.0f%% sep", pct), colStall
	case sep >= sepOnset:
		return fmt.Sprintf("separating %.0f%%", pct), colWarn
	default:
		return fmt.Sprintf("attached %.0f%%", pct), colOK
	}
}

// drawString renders one line of UI text with its top-left at (x,y).
func drawString(dst *ebiten.Image, s string, x, y float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, uiFace, op)
}

// Layout fixes the logical resolution to the window size.
func (g *Game) Layout(_, _ int) (int, int) {
	return winW, winH
}
