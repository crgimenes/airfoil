# higher-inlet-speed

Type: question/discussion issue only, no PR (asking about feasibility before touching solver constants).

## Issue title
Room to raise the inlet speed slider's max, or is 0.15 a real stability ceiling?

## Issue body
Separate from #21 (Pi 5 performance) -- this one's about the
physics/stability side rather than raw speed, though I suspect the two are
related.

The inlet speed slider is currently `spdMin = 0.02` / `spdMax = 0.15`
(lattice units), with the comment noting this is "the solver stays stable"
range, and `tau = 0.6` fixed. I'd like visitors to be able to push the flow
noticeably faster than that -- but before touching any of these constants, I
wanted to ask whether 0.15 is close to a real ceiling or just a conservative
default.

From reading `lbm/lbm.go`, it looks like there's already a documented safety
net for exactly this: local speeds get clamped past `uMax` (~0.2) because
"extreme settings... push local speeds past what BGK at this tau can
integrate, and the run dissolves into NaNs." At `spdMax = 0.15`, a blocking
body can apparently already push local speeds close to that clamp (the
broadside-body comment mentions ~70% channel blockage). So it looks like
BGK's single-relaxation-time collision operator itself may be the limiting
factor here, not just an arbitrary slider range -- but I don't know the
solver well enough to know whether:

- a lower `tau` (thinner boundary layers, different stability envelope) would
  buy meaningfully more headroom without other tradeoffs,
- a different collision operator (MRT/TRT instead of BGK) would be needed to
  go meaningfully faster without leaning on the clamp constantly, or
- 0.15 is basically already close to where useful (not just numerically
  finite) results stop, and the clamp is more of a last-resort safety net
  than something to design around.

And separately from stability: even if there's numerical headroom, would
higher `u0` meaningfully increase the per-step cost, on top of whatever's
going on with #21? I don't have a good intuition for whether
speed itself costs more compute in LBM the way, say, grid resolution or
substep count would.

Wanted to ask before guessing at solver constants I don't fully understand
the stability implications of.
