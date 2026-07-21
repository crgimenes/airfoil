# merge-clipboard

Branch: feature/merge-clipboard
Recommended: open the issue first, then the PR (new editor tool).

## Issue title
Editor: join two open outlines drawn separately into one shape

## Issue body
If you draw a shape's two halves separately with the pen tool (say, an upper
and lower surface, each left open rather than closed), there's currently no
way to join them into one solid body short of redrawing everything as a
single continuous path.

I'd like a "merge" action: copy one open object to the clipboard, select the
other, and merge — joining their two open ends into one closed outline. It
shouldn't matter which direction either half was drawn in; the merge should
figure out which pairing of ends actually belongs together.

What I'd want to see on screen:
- A keyboard shortcut (Cmd+M felt natural, alongside the existing Cmd+C/V/X
  object clipboard) and a matching Edit menu item.
- Selecting the result afterward shows one closed shape, not two.

I've already got a working version of this (`joinChains`, below), so a PR
will follow right after this issue.

---

## PR title
Add Merge with Clipboard: join two open outlines end-to-end

## PR body

(Following up from #7 — a bit more background there on who I am.)
This came up while assembling that same traced wing profile: I'd drawn the
upper and lower surfaces separately and needed to join them into one shape.

## Summary
Cmd+M (Geometry mode) or **Edit → Merge with Clipboard** joins the clipboard
object's outline onto the selected object's, replacing the selection with the
closed result.

`joinChains` tries all four end-to-end orientations (each chain forward or
reversed) and picks whichever brings the loose ends closest together, so it
doesn't matter which direction either half was drawn in.

## Why
See #13. A shape drawn as two open halves with the pen tool had
no way to become one solid body other than redrawing it as a single
continuous path.

## How to use it
1. Select one half, **Cmd+X** (Cut — copies to clipboard and removes it).
2. Select the other half, **Cmd+M** (Merge with Clipboard).
3. The selected object becomes one closed outline combining both.

The clipboard object itself is untouched by the merge; Cut (not Copy) first
if it shouldn't remain in the scene afterward.

## Test plan
- [x] `go build ./... && go vet ./... && golangci-lint run ./... && go test ./...`
      (includes `TestJoinChainsPicksClosestOrientation`)
- [ ] Draw two open halves in opposite directions (e.g. one LE→TE, one
      TE→LE); merge; confirm one clean closed loop, not a crossed one.
- [ ] Confirm Cmd+Z undoes the merge.
