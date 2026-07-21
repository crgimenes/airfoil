# pi5-performance

Type: question/discussion issue only, no PR (not a code contribution -- asking for tuning ideas).

## Issue title
Choppy/halting simulation on Raspberry Pi 5 -- performance tuning ideas?

## Issue body
I'm retrofitting a 1949 desktop wind-tunnel display with a Raspberry Pi 5 for
a real exhibit, and porting kutta over went smoothly -- it builds and runs
fine -- but the simulation itself is slow and halting on the Pi: not the
smooth flow I get on my Mac, more of a stutter/stall. Before giving up on the
Pi and switching to a headless/mini-PC setup instead, I wanted to ask if you
had any tuning ideas, since you know the solver and renderer far better than
I do.

What I've ruled out so far:
- **Not software rendering.** `glxinfo | grep renderer` confirms direct
  rendering via the real V3D 7.1.10.2 GPU (Mesa), not a software fallback.
- **Not thermal throttling.** `vcgencmd measure_temp` / `get_throttled` show
  57.6°C and `throttled=0x0` under load.
- **Not just a display/VNC artifact.** Same behavior over VNC and on a real
  HDMI monitor with VNC closed.

What I've tried, which helped some but didn't fix it:
- The streamline overlay was rebuilding and re-tessellating its
  `vector.Path` every frame with `AntiAlias: true` and `LineJoinRound` --
  genuinely expensive on weaker hardware. Throttled the RK2 integration to
  every 3rd frame (cached in between) and switched to `LineJoinBevel` +
  `AntiAlias: false`. This smoothed out the streamlines specifically, but the
  simulation as a whole is still halting.
- Added a `-substeps` flag to test whether the LBM solver's per-frame
  substep count was the bottleneck. Even at `-substeps 1` it's still slow, so
  substep count alone isn't the explanation.

What I haven't tried yet, and would love a sanity check on before I sink more
time in:
- `lbm.Solver.Step()` looks single-threaded to me (no goroutines) -- the
  Pi 5's per-core performance is a lot weaker than a modern laptop's, so this
  is my leading suspect, but I don't know the solver's internals well enough
  to be sure parallelizing it is safe/correct.
- Grid resolution (`gridW`/`gridH`) is the other obvious lever, but I know
  some of the separation/stall thresholds elsewhere in the code are
  calibrated against the current grid size, so I didn't want to touch that
  blind.
- Smoke particle count (`nParticles`) and `pixScale` are smaller, more
  contained things I haven't tried yet either.

Any pointers on which of these is actually worth chasing (or a completely
different bottleneck I'm not seeing) would be a big help. I'd like to keep
this running on the Pi if there's a reasonable path -- it's the right form
factor for the exhibit -- but wanted to ask before spending more time on a
guess-and-check approach.
