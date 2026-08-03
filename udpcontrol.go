package main

import (
	"bufio"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// udpRebindInterval is how often a multicast listener is closed and reopened
// to force a fresh OS-level group join; see startUDPControl.
const udpRebindInterval = 30 * time.Second

// startUDPControl opens a UDP listener at addr and applies incoming
// slider/display commands as they arrive, for driving kutta from external
// hardware -- e.g. an ESP32 with rotary encoders and switches. UDP is the
// only one of the common options (WebSockets, UDP, MQTT) that needs no new
// dependency and no broker process, matching this project's
// single-executable, family-stack rule. Nothing is sent back; this is
// receive-only.
//
// addr is a plain unicast listen (":9000" for all interfaces, "1.2.3.4:9000"
// for one) or a multicast group address ("239.192.1.1:9000" -- pick from the
// 239.0.0.0/8 administratively-scoped range, RFC 2365, reserved for exactly
// this kind of private/local use), chosen automatically by whether the host
// parses as a multicast IP.
//
// Channels: AOA, SPD and CTRL (angle of attack, inlet speed, control-surface
// deflection in degrees; all numeric), GLOW, STREAMLINES, PARTICLES and LABEL
// (0 or 1), MODE (speed, vorticity, or pressure), and DEMO (seconds of no
// real input before the sim gently wanders on its own; 0 disables it),
// mirroring the -demo flag exactly.
func (g *Game) startUDPControl(addr string) error {
	conn, err := listenUDPControl(addr)
	if err != nil {
		return err
	}
	log.Printf("kutta: UDP control listening on %s", conn.LocalAddr())
	go g.udpControlSupervisor(addr, conn)
	return nil
}

// udpControlSupervisor runs the read loop on conn and, for a multicast
// address, periodically closes and reopens the listener. Multicast group
// membership can be silently dropped by the OS with no error and nothing to
// observe on the existing connection (a Wi-Fi power-save cycle or a roam
// event, in practice) -- Go's net package exposes no way to check or refresh
// membership on an already-open socket, so a full re-bind, which forces a
// fresh join, is the fix that needs no new dependency. Unicast never drops
// membership (there is none), so it only ever runs the plain read loop.
func (g *Game) udpControlSupervisor(addr string, conn net.PacketConn) {
	if !isMulticastAddr(addr) {
		g.udpControlLoop(conn)
		return
	}
	for {
		done := make(chan struct{})
		go func(c net.PacketConn) {
			g.udpControlLoop(c)
			close(done)
		}(conn)

		select {
		case <-done:
			// The read loop only returns on a real socket error; rebinding
			// below is also the recovery path for that, not just the timer.
		case <-time.After(udpRebindInterval):
			// Closing is how the read loop is unblocked, so a close error has
			// nowhere useful to go: the socket is being replaced regardless.
			_ = conn.Close()
			<-done
		}

		newConn, err := listenUDPControl(addr)
		if err != nil {
			log.Printf("kutta: UDP control: rebind failed, retrying: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		conn = newConn
	}
}

// listenUDPControl opens addr for receiving: a multicast group join if the
// host is a multicast IP, a plain unicast/wildcard listen otherwise. Both
// return a net.PacketConn, so the caller (and udpControlLoop) don't need to
// care which one they got.
func listenUDPControl(addr string) (net.PacketConn, error) {
	if isMulticastAddr(addr) {
		gaddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return nil, err
		}
		return net.ListenMulticastUDP("udp", nil, gaddr) // nil: let the OS pick the interface
	}
	return net.ListenPacket("udp", addr)
}

// isMulticastAddr reports whether addr's host is a multicast IP (224.0.0.0/4
// for IPv4, ff00::/8 for IPv6 -- net.IP.IsMulticast covers both), so -udp can
// switch between a plain listen and a multicast join automatically.
func isMulticastAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsMulticast()
}

// udpControlLoop reads packets until the socket errors (typically only on
// shutdown) or the process exits; each packet may hold one or more
// newline-separated messages.
func (g *Game) udpControlLoop(conn net.PacketConn) {
	buf := make([]byte, 512)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			log.Printf("kutta: UDP control: %v", err)
			return
		}
		sc := bufio.NewScanner(strings.NewReader(string(buf[:n])))
		for sc.Scan() {
			g.applyControlMessage(sc.Text())
		}
	}
}

// applyControlMessage parses one line and, if it is valid, enqueues the
// matching update to run on the game goroutine -- the UDP loop is its own
// goroutine, so game state can't be touched from it directly. Each channel
// parses its own value: AOA/SPD are floats, GLOW/STREAMLINES are 0 or 1, MODE
// is a field name (speed, vorticity, pressure).
func (g *Game) applyControlMessage(line string) {
	channel, value, ok := parseControlMessage(line)
	if !ok {
		log.Printf("kutta: UDP control: bad message %q", line)
		return
	}
	switch channel {
	case "AOA":
		v, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: AOA wants a number, got %q", value)
			return
		}
		g.enqueue(func() { g.setAlpha(v) })
	case "SPD":
		v, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: SPD wants a number, got %q", value)
			return
		}
		g.enqueue(func() { g.setSpeed(v) })
	case "CTRL":
		v, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: CTRL wants a number, got %q", value)
			return
		}
		// setControl clamps to +-controlLimit itself and is a harmless no-op
		// when the loaded scene has no object marked Control, so the sender
		// doesn't need to know whether one exists.
		g.enqueue(func() { g.setControl(v) })
	case "GLOW":
		on, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: GLOW wants 0 or 1, got %q", value)
			return
		}
		g.enqueue(func() { g.glow = on != 0 })
	case "STREAMLINES":
		on, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: STREAMLINES wants 0 or 1, got %q", value)
			return
		}
		g.enqueue(func() { g.streamlines = on != 0 })
	case "PARTICLES":
		on, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: PARTICLES wants 0 or 1, got %q", value)
			return
		}
		g.enqueue(func() { g.showParticles = on != 0 })
	case "LABEL":
		on, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: LABEL wants 0 or 1, got %q", value)
			return
		}
		g.enqueue(func() { g.showLabel = on != 0 })
	case "MODE":
		fm, fok := parseFieldMode(value)
		if !fok {
			log.Printf("kutta: UDP control: MODE wants speed, vorticity, or pressure, got %q", value)
			return
		}
		g.enqueue(func() { g.mode = fm })
	case "DEMO":
		v, perr := strconv.ParseFloat(value, 64)
		if perr != nil {
			log.Printf("kutta: UDP control: DEMO wants a number of seconds (0 disables), got %q", value)
			return
		}
		g.enqueue(func() {
			g.demoIdleSec = v
			if v <= 0 {
				// Hand control back immediately rather than freezing mid-wander:
				// updateDemo skips everything once demoIdleSec <= 0, so without
				// this it wouldn't apply further changes but also wouldn't clear
				// the flag it's using to track that state.
				g.demoActive = false
			}
		})
	default:
		log.Printf("kutta: UDP control: unknown channel %q", channel)
	}
}

// parseControlMessage splits one "CHANNEL VALUE" line (whitespace-separated,
// case-insensitive channel) into the channel and the raw value string, e.g.
// "AOA 12.5" -> ("AOA", "12.5"). It is a pure function so the wire protocol
// can be tested headlessly, without a socket; each channel parses its own
// value since they aren't all numbers (MODE is a name).
func parseControlMessage(line string) (channel, value string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return "", "", false
	}
	return strings.ToUpper(fields[0]), fields[1], true
}
