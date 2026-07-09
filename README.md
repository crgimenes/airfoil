# kutta - 2D wind tunnel

![kutta](https://github.com/crgimenes/kutta/blob/trunk/fixtures/kutta.gif)

A 2D wind tunnel for aeromodelers and anyone who likes watching air misbehave.
It streams a flow past an airfoil and draws the speed field, the vorticity, smoke
streaklines and the lift/drag vectors while you turn the angle of attack. You can
also draw your own shape, cut a wing into wing + flap, animate the control
surfaces, and watch the wake fall apart when you push the angle too far.

It is **qualitative, not validated CFD**. Lattice units stand in for physical
ones, so it gets the *shape* of the flow right (stagnation point, wake, suction
over the top, separation as the angle grows) and the actual numbers wrong. Use it
for intuition and for a demo that looks good on a projector, not for sizing a real
wing.

Written in Go with [Ebitengine](https://ebitengine.org). One executable, nothing
to install alongside it.

## Download (no Go required)

Grab a prebuilt binary from the
[latest release](https://github.com/crgimenes/kutta/releases/latest). You do
**not** need Go or any developer tools.

| System | File to download |
| --- | --- |
| macOS (Intel or Apple Silicon) | `kutta-darwin-universal.zip` |
| Windows (64-bit, most common) | `kutta-windows-amd64.exe` |
| Windows (older 32-bit) | `kutta-windows-386.exe` |
| Windows (ARM) | `kutta-windows-arm64.exe` |
| Linux (Intel/AMD 64-bit) | `kutta-linux-amd64.gz` |
| Linux (ARM 64-bit) | `kutta-linux-arm64.gz` |

### macOS

With [Homebrew](https://brew.sh), one command:

```bash
brew install --cask crgimenes/tap/kutta
```

Or by hand:

1. Download `kutta-darwin-universal.zip` and double-click it to unzip. You get
   `kutta.app`.
2. Move `kutta.app` to your **Applications** folder.
3. Double-click it to run.

The app is signed and notarized by Apple, so it opens normally. The single
universal build runs on both Intel and Apple Silicon, so there is no architecture
to choose.

If macOS says **"kutta.app is damaged and can't be opened"** or complains about
an unidentified developer:

- Make sure the download finished, and unzip before opening (do not run the app
  from inside the `.zip`). Re-download if in doubt.
- Right-click `kutta.app`, choose **Open**, then confirm with **Open** in the
  dialog.
- If it still refuses, open **Terminal** and run (adjust the path if you did not
  move it to Applications):

  ```bash
  xattr -dr com.apple.quarantine /Applications/kutta.app
  ```

### Windows

Download the `.exe` for your machine and double-click it. Windows SmartScreen may
warn you because the app is not from the Microsoft Store: click
**More info → Run anyway**.

### Linux

Download the matching `.gz`, then decompress and run:

```bash
gunzip kutta-linux-amd64.gz
chmod +x kutta-linux-amd64
./kutta-linux-amd64
```

## Run from source

With Go installed:

```bash
go run .
```

On macOS and Windows there is nothing else to install. On **Linux**, Ebitengine
needs Cgo and the system development libraries, so build with `CGO_ENABLED=1`
after installing the packages from the
[Ebitengine install guide](https://ebitengine.org/en/documents/install.html) (on
Debian/Ubuntu: `libgl1-mesa-dev`, `libasound2-dev`, `libxcursor-dev`, `libxi-dev`,
`libxinerama-dev`, `libxrandr-dev`, `libxxf86vm-dev`, `pkg-config`).

## Controls

| Key | Action |
| --- | --- |
| ↑ / ↓ | angle of attack |
| Tab | cycle NACA profile |
| V | speed / vorticity / pressure field |
| S | streamlines |
| G | glow / bloom |
| `[` `]` | inlet speed |
| Space | pause / resume |
| N | single step (while paused) |
| R | reset the flow |
| E | open the editor |
| O | open an `.afoil` scene |
| Cmd+O | open (native file dialog) |
| Cmd+S | save |
| Cmd+Shift+S | save as |
| L | play / pause the animation |
| Esc | back to the foil |

Type a code in the field at the top (`2412`, `0012`, `23012`, ...) to pick any
NACA 4- or 5-digit foil, not just the ones on Tab. And if you type `neko` in that
field, you get a cat. It has terrible aerodynamics. That is the point.

## The editor

Press **E**. Draw a shape with the pen, drag its vertices, and curve an edge by
pulling out a Bézier handle. Cut a closed outline at a vertex and rejoin two loose
ends to split one airfoil into separate parts, which is how you turn a wing into
wing + flap that each follow the real profile.

**Tab** switches between geometry and animate. In animate you scrub a timeline and
drop a keyframe for each part's pose, so a flap can deploy and retract on a loop
while the flow keeps running. Scenes save as `.afoil`, a small
[Filo](https://github.com/crgimenes/filo) s-expression file; see `examples/` for a
few (`flap.afoil`, `neko.afoil`).

Geometry does not have to be drawn by hand or be an airfoil at all. **File →
Import SVG…** — or simply **dragging an `.svg` or `.afoil` file onto the
window** — loads a vector drawing as a scene: one object per subpath, with
curves flattened and the group scaled onto the grid. A car silhouette, a
building section or a bridge deck from Inkscape or Illustrator drops straight
into the tunnel. (On Windows and Linux the native file dialogs are not wired
up yet, so drag and drop is the way to open files there.) Scene files can
also reference drawings directly with
`(svg "path" chord leadX leadY [subpath])`, alongside the existing
`(naca "2412" ...)` and `(dat "file.dat" ...)` sources. Transforms are not
interpreted (flatten them before exporting) and holes become solid.

## How it works

- **Solver** (`lbm`): a 2D Lattice-Boltzmann method (D2Q9, BGK). An open channel
  with a free stream from the left; the body is a no-slip wall via half-way
  bounce-back. Forces come from a pressure integral over the body faces, so it
  captures form drag and lift. Skin friction is ignored, which means drag is
  understated. That is a known limit, not a bug to file.
- **Geometry** (`foil`): NACA 4- and 5-digit airfoils straight from their
  closed-form equations. A whole catalog with nothing stored on disk.
- **Scenes** (`scene`, `sceneio`): multi-object shapes with per-vertex Bézier
  handles, cuts, and keyframe animation, read and written as `.afoil`.
- **Visualization** (`viz`): color maps for the scalar fields and a smoke-tracer
  particle system pushed around by the velocity field.
- **App** (`main.go`, `game.go`, `editor.go`): the Ebitengine loop, the editor,
  and the native macOS/Windows menu (via [glaze](https://github.com/crgimenes/glaze)).

`lbm`, `foil`, `scene` and `viz` carry no rendering dependency and are unit tested
headlessly. `cmd/snapshot` renders the fields to PNG without a GPU, which is how
the physics gets sanity-checked: lift rising with angle of attack, the drag
bucket, the force signs coming out right.

## License

See [LICENSE](LICENSE).

---

## More of my projects

- [minigui](https://github.com/crgimenes/minigui): a tiny immediate-mode GUI for Ebitengine.
- [filo](https://github.com/crgimenes/filo): a small scripting language safe to embed in Go programs.
- [glaze](https://github.com/crgimenes/glaze): WebView desktop apps in Go, cgo-free.
- [neko](https://github.com/crgimenes/neko): the classic desktop cat chasing your pointer, in Go.

More at [github.com/crgimenes](https://github.com/crgimenes) and [crg.eti.br](https://crg.eti.br).
