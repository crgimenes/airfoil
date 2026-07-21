# udp-control

Branch: feature/udp-control
Opening issue + PR together, noting in the issue that the PR is ready and
Cesar should feel free to reject or request changes before merging.
Note: soft dependency on control-surface (the CTRL channel) -- consider submitting after that one lands.
Note: rebased on trunk since this was cut, to merge the new `-udp` flag
alongside trunk's own new `-scene`/`-fullscreen`/`-hidecontrols` flags --
purely additive, no design changes. See "Built on trunk's recent changes"
in the PR body.

## Issue title
External hardware control (rotary encoders, etc.) over UDP

## Issue body
I'd  like to control kutta's sliders from physical hardware — an
ESP32 with a couple of rotary encoders, say — rather than mouse/keyboard,
for a kiosk-style installation.

Of the usual options for this (WebSockets, UDP, MQTT), UDP seems like the
only one that fits this project's constraints: it's pure stdlib `net`, no new
Go dependency, and no broker/external process to run alongside the app —
WebSockets would need a new library, MQTT needs a broker process, which
conflicts with "one executable, nothing to install alongside it."

What I'd want to see on screen (or rather, in the terminal):
- A `-udp :9000`-style flag opening a receive-only UDP listener.
- A tiny text protocol: one line per message, e.g. "AOA 12.5" or "SPD 0.08",
  driving the matching slider live.
- The same channel for the display settings otherwise only reachable via
  keyboard -- glow/streamlines on/off, and switching the speed/vorticity/
  pressure field display -- so a physical switch or button on the hardware
  side can flip those live too, not just at startup.
- No response/ack needed to start — this is v1, receive-only.
- Ideally, no fixed/static IP needed on the kutta side, and no pairing step
  between it and the hardware -- just both devices on the same WiFi. A
  multicast address (e.g. `239.192.1.1:9000`, from the administratively-scoped
  239.0.0.0/8 range reserved for exactly this kind of private/local use, RFC
  2365) does this: the hardware sends to the group without knowing kutta's
  address at all, and kutta joins that group regardless of whatever IP DHCP
  gave it.

This is a genuinely new kind of surface for kutta (a network listener), so
I've opened a PR with a working version alongside this issue -- please feel
free to reject it, or ask for changes, before merging if it's not the right
fit.

---

## PR title
Add UDP control for external hardware (ESP32 rotary encoders, etc.)

## PR body


(Following up from #7 — a bit more background )
The 1949 tunnel's physical AoA and Velocity knobs are exactly what I'm
planning to wire back in, with modern hardware behind them.

## Summary
`kutta -udp :9000` starts a receive-only UDP listener that drives the sim
from simple newline-delimited text messages -- "AOA 12.5" / "SPD 0.08" for
the sliders, "GLOW 1" / "STREAMLINES 0" to toggle display options, "MODE
vorticity" to switch the field display -- aimed at physical control panels
rather than mouse/keyboard.

## Why
See #19. UDP is the only realistic option of the usual three:
zero new Go dependency, no broker/external process, matching this project's
single-executable, family-stack-only rule.

## Built on trunk's recent changes
Trunk added its own `-scene`, `-fullscreen`, and `-hidecontrols` startup
flags while this branch was in progress, all declared in the same spot in
`main.go` as `-udp`. Rebased to merge them together -- `-udp` sits alongside
the others with no interaction between them (it only wires up
`startUDPControl`, nothing that touches `g.clean`/`g.startFullscreen`/scene
loading), so this was a purely additive conflict, not a design one.

## How to use it
    kutta -udp :9000              # unicast, all interfaces
    kutta -udp 192.168.1.50:9000  # unicast, one interface only
    kutta -udp 239.192.1.1:9000     # multicast group -- auto-detected

Then send newline-delimited messages, e.g. from a shell:

    echo "AOA 15" | nc -u -w1 127.0.0.1 9000
    echo "SPD 0.1" | nc -u -w1 127.0.0.1 9000
    echo "GLOW 0" | nc -u -w1 127.0.0.1 9000
    echo "STREAMLINES 1" | nc -u -w1 127.0.0.1 9000
    echo "MODE vorticity" | nc -u -w1 127.0.0.1 9000

## Design notes
- Incoming messages are applied via the existing `enqueue()` mechanism (the
  same one native-menu clicks already use), since the UDP listener runs on
  its own goroutine and can't touch game state directly.
- `parseControlMessage` splits a line into its channel and raw value string;
  each channel parses its own value (AOA/SPD as floats, GLOW/STREAMLINES as
  0/1, MODE as a field name), since not everything is numeric. It's a pure
  function with its own test, so the wire protocol is covered without a
  socket. A bad value for any channel is logged and ignored, not fatal --
  verified live by sending a malformed message mid-session: logged, ignored,
  no crash.
- AOA, SPD, GLOW, STREAMLINES, and MODE are wired up (all have always
  existed); a CTRL channel for a control-surface slider is a natural
  follow-up once one exists to drive -- happy to add that as a small
  follow-up commit here once/if that lands.
- `-udp` auto-detects a multicast address (`net.IP.IsMulticast()`, covering
  both 224.0.0.0/4 for IPv4 and ff00::/8 for IPv6) and joins that group with
  `net.ListenMulticastUDP` instead of a plain unicast listen. Both return a
  `net.PacketConn`, so the read loop and message parsing don't need to know
  which one they got. This is aimed at a same-WiFi setup with no fixed IP on
  either side: the sending hardware doesn't need to know kutta's address at
  all, just the group's.
- Tested locally with `nc -u`: sustained AOA/SPD/GLOW/STREAMLINES/MODE
  traffic (including a continuously varying feed, not just one-shot values)
  against both the plain build and the combined kiosk+control-surface build
  below, with no crashes or dropped state.

## Test plan
- [x] `go build ./... && go vet ./... && golangci-lint run ./... && go test ./...`
      (includes `TestParseControlMessage`, `TestApplyControlMessageTogglesAndMode`,
      `TestIsMulticastAddr`)
- [x] `kutta -udp :9000`, confirm the "listening on" log line. (verified live)
- [x] Send `AOA 15` and `SPD 0.1` via `nc -u`; confirm both sliders update
      live with no crash. (verified live, including a continuous varying feed)
- [x] Send `GLOW 0`, `STREAMLINES 1`, `MODE vorticity`; confirm each takes
      effect live. (verified live)
- [x] Send a malformed message (bad channel, bad value); confirm it's logged
      and ignored, not fatal. (verified live, incidentally, via a malformed
      MODE value)
- [ ] `kutta -udp 239.192.1.1:9000`; confirm the log line shows the multicast
      group, and a message sent to that group (e.g. `echo "AOA 10" | nc -u
      -w1 239.192.1.1 9000`) is applied.
