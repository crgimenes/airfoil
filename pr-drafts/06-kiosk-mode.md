# kiosk-mode

Branch: feature/kiosk-mode
Opening issue + PR together, noting in the issue that the PR is ready and
Cesar should feel free to reject or request changes before merging.
Note: since this branch was cut, trunk grew its own basic `-fullscreen`/
`-hidecontrols` flags (`g.clean`, `g.startFullscreen`). This branch has been
rebased on top of that and reuses that state rather than adding a parallel
one -- see the "Built on trunk's recent changes" section in the PR body.

## Issue title
Kiosk mode: fullscreen, panel-free display for public installations

## Issue body
I'd like to run kutta as an unattended public display (a booth, a classroom
projector, a fixed installation) — fullscreen, no menu, no editor, nothing a
passerby could accidentally break into the shape editor or a file dialog
with.

I see `-fullscreen` and `-hidecontrols` already do the fullscreen/hide-panels
part — this proposes building the rest of a kiosk experience on top of that:

What I'd want to see on screen:
- A `-kiosk` startup flag: fullscreen, just the flow, menu collapsed to
  basically nothing. (Under the hood this is just `-fullscreen -hidecontrols`
  plus the items below, not a new display mode.)
- A `-kiosk-controls` variant that keeps the AoA/speed/control sliders
  visible and usable, for a touch- or mouse-driven kiosk.
- A way to get back to the normal window without relaunching (for
  maintenance) and a way back into kiosk mode without a relaunch either --
  today `-hidecontrols`/`-fullscreen` are startup-only.
- While in kiosk mode: no Open/Save dialogs, no way into the shape editor —
  nothing that reaches the filesystem or the authoring tools. (`-hidecontrols`
  today only hides the panels; the hotkeys/menu underneath are still live.)
- Startup flags for the display settings that are otherwise only reachable
  via keyboard -- glow, streamlines, and the speed/vorticity/pressure field
  display -- since a kiosk has no keyboard to press G/S/V on.

Given this is a bigger, more opinionated addition than most of the other
things I've sent over, I've gone ahead and opened a PR with a working version
alongside this issue -- please feel free to reject it, or ask for changes,
before merging if the design doesn't fit.

---

## PR title
Add kiosk mode: fullscreen, panels hidden, two variants

## PR body

(Following up from #7 — a bit more background there on who I am.)
This is really the core of the exhibit retrofit I mentioned there: a public,
unattended display for students to play with.

## Summary
`kutta -kiosk` starts fullscreen with the menu collapsed to just
Quit/Exit-kiosk, no side/bottom panels, no shape editor — just the flow.
`-kiosk-controls` keeps the AoA/speed/control sliders visible and usable,
docked under the viewport, for a touch/mouse kiosk.

`Ctrl+Shift+K` toggles both directions from anywhere (chosen over F11/
Ctrl+Alt+K since it doesn't collide with any default OS shortcut on macOS,
Windows, or Linux). **View → Enter Kiosk Mode (with Controls)** does the same
without a flag or relaunch.

Also adds `-glow`, `-streamlines`, and `-mode` startup flags, for setting the
display options that are otherwise only reachable via keyboard (G, non-meta
S, V) -- useful for a kiosk with no keyboard attached, though they work in
the normal window too.

## Built on trunk's recent changes
Trunk grew its own basic kiosk support (`-fullscreen`/`g.startFullscreen`,
`-hidecontrols`/`g.clean`) while this branch was in progress -- both flags
still work exactly as before, unchanged. Rather than add a second, parallel
`g.kiosk` bool alongside them, this branch was rebased to reuse that same
state:
- `enterKiosk`/`exitKiosk`/`toggleKiosk` (in the new `kiosk.go`) set
  `g.clean` instead of a separate flag, so `-hidecontrols`, `-kiosk`, and the
  View menu / hotkey all drive the same underlying mode.
- Entering kiosk mode now requests fullscreen through the existing
  `startFullscreen`/`fsCountdown` deferred-apply path instead of calling
  `ebiten.SetFullscreen(true)` directly. That path exists to work around a
  macOS bug where entering fullscreen before the first frame renders leaves
  the window black until a resize; `-kiosk` at startup hits the exact same
  case `-fullscreen` already fixed, so this reuses that fix instead of
  reintroducing the bug. Toggling at runtime (already past the window's first
  frame) still applies on the very next frame -- no perceptible delay.
- `kioskControls` is a genuinely new field (trunk's version has no
  controls-visible variant), so it's additive rather than a rename.

## How to use it
    kutta -kiosk                    # fullscreen, flow only
    kutta -kiosk -kiosk-controls    # fullscreen, flow + sliders
    kutta -kiosk -glow=false -streamlines -mode vorticity

Ctrl+Shift+K exits back to the normal window (and re-enters kiosk mode if
pressed again, remembering the controls variant last used). From the normal
window, View → Enter Kiosk Mode / Enter Kiosk Mode (with Controls) does the
same without any flag.

`-glow` and `-streamlines` default to the app's normal defaults (true and
false respectively), so omitting either leaves current behavior unchanged;
`-mode` accepts speed, vorticity, or pressure (default speed).

## Design notes
- While active, Escape (clear scene)/Open/Save/entering the shape editor are
  all blocked -- deliberate, since a public/unattended kiosk shouldn't let a
  visitor reach the filesystem or the editor. This is new relative to
  `-hidecontrols`, which only hides the panels and leaves those hotkeys live.
- `Layout()` reports a reduced logical canvas (just the viewport, or viewport
  plus a slider strip) in kiosk mode; Ebiten scales/letterboxes that to fill
  the real display, so no per-resolution layout code was needed.
- Tested locally (interactively, on a real display) in every combination:
  plain `-kiosk`, `-kiosk -kiosk-controls`, the Ctrl+Shift+K toggle in both
  directions, Escape/Open/Save/Editor lockout while active, and
  `-glow`/`-streamlines`/`-mode` at startup -- and together with the
  control-surface and udp-control branches (a scene with a marked control
  object, `-kiosk-controls` showing its slider, driven live over `-udp`).

## Test plan
- [x] `go build ./... && go vet ./... && golangci-lint run ./... && go test ./...`
- [x] `kutta -kiosk`: fullscreen, no panels, menu bar reduced. (verified live)
- [x] `kutta -kiosk -kiosk-controls`: sliders visible and functional. (verified
      live; caught and fixed a bug where the slider strip drew empty)
- [x] Ctrl+Shift+K exits to the normal window; pressing it again re-enters
      kiosk mode with the same controls variant. (verified live)
- [x] While in kiosk mode: O, Cmd+S, Cmd+Shift+S, E, and Escape (with a scene
      loaded) are all no-ops. (verified live)
- [x] `kutta -glow=false -streamlines -mode pressure`: starts with glow off,
      streamlines on, pressure field showing. (verified live)
- [ ] `kutta -fullscreen` and `kutta -hidecontrols` on their own, unchanged
      from trunk's current behavior.
