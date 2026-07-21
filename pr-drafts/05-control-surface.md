# control-surface

Branch: feature/control-surface
Recommended: open the issue first, then the PR (new tool/behavior).
Note: rebased on trunk since this was cut, to adopt the broken-outline-style
PR's `PolygonAt`/`strokeClosed(width)` API -- see "Built on trunk's recent
changes" in the PR body.

## Issue title
Live slider-driven control surface (independent of keyframe animation)

## Issue body
Right now, animating a control surface (a flap, aileron, etc.) means
authoring keyframes and scrubbing/playing a timeline. I'd like a second way
to drive one: a real-time slider, like the existing angle-of-attack and inlet
-speed sliders, that directly sets an object's deflection with no keyframes
involved — closer to a live control stick than an authored animation.

What I'd want to see on screen:
- A way to mark one object in the scene as "the" live-controlled one (a
  checkbox in the editor's object list felt natural).
- A third slider next to AoA/Speed in the simulator, appearing only when a
  scene has a marked object, driving its deflection live — including while
  the timeline is paused.
- Only one object controllable at a time to start; happy to leave
  multi-channel (aileron + elevator + rudder, say) as a later idea.

I've already got a working version of this, so a PR will follow right after
this issue.

---

## PR title
Add a live control-surface slider

## PR body

(Following up from #7 — a bit more background there on who I am.)
The 1949 tunnel I mentioned there had a physical control-surface demo too --
this is its modern, on-screen equivalent.

## Summary
Marking an object "Control Surface" (a checkbox in the editor's object panel)
lets a bottom-panel slider drive its deflection directly and in real time,
bypassing its keyframe track entirely while simulating.

Also reorders the existing sliders (speed, then AoA, then control) so AoA and
the new control slider sit next to each other, and grows the bottom-right
panel (156px -> 210px) to fit the third row.

## Why
See #15. Keyframe animation is the right tool for an authored,
repeatable motion; this adds the complementary real-time-control path for
interactively exploring a deflection angle.

## How to use it
1. In the editor, select an object, check **Control Surface** in the side
   panel (this is exclusive -- checking it on one object clears any other).
2. Back in the simulator, a **Control surface** slider appears (±40 deg),
   alongside AoA/Speed, and updates the flow live, even while paused.

Included: `examples/planewing.afoil` (with `examples/open_airplane_profile.svg`,
the source it was imported from via File → Import SVG…) — a real scene where
the flap (`flap`) is marked Control Surface.

## Built on trunk's recent changes
Trunk merged the broken-outline-style PR (#7) while this branch was in
progress, which renamed `g.objectPolygon(o, t)` → `o.PolygonAt(t)` and gave
`strokeClosed` a new `width` parameter for broken-vs-solid outlines. Rebased
onto that: `drawOutline` now calls the live control-surface override
(`objectPolygon`, which falls through to `o.PolygonAt(t)` for every other
object) and passes the new `width` for the broken/solid split, so both
features apply together rather than one silently overriding the other.

## Design notes
- Persists as a `(control)` tag in `.afoil` files.
- It's a full override, not additive: while simulating, the live slider
  replaces the object's keyframe pose entirely. The editor's own Animate-mode
  timeline is untouched -- you can still author/preview keyframes there: they
  just won't apply during simulation while Control is checked.

## Test plan
- [x] `go build ./... && go vet ./... && golangci-lint run ./... && go test ./...`
      (includes `TestControlRoundTrip`)
- [x] Mark an object Control, confirm the slider appears and moves the flow
      live, including while paused. (verified live)
- [ ] Save/reload the scene; confirm the Control flag and marked object
      persist.
- [x] Mark a second object Control; confirm the first is automatically
      unmarked. (verified live)
