package main

import (
	"bufio"
	"log"
	"net"
	"strconv"
	"strings"
)

// startUDPControl opens a UDP listener at addr (e.g. ":9000") and applies
// incoming slider/display commands as they arrive, for driving kutta from
// external hardware -- e.g. an ESP32 with rotary encoders and switches. UDP
// is the only one of the common options (WebSockets, UDP, MQTT) that needs no
// new dependency and no broker process, matching this project's
// single-executable, family-stack rule. Nothing is sent back; this is
// receive-only.
//
// Channels: AOA and SPD (angle of attack, inlet speed, both numeric), GLOW
// and STREAMLINES (0 or 1), and MODE (speed, vorticity, or pressure). A
// control-surface channel is a natural follow-up once there's a live
// control-surface slider for it to drive.
func (g *Game) startUDPControl(addr string) error {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	log.Printf("kutta: UDP control listening on %s", conn.LocalAddr())
	go g.udpControlLoop(conn)
	return nil
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
	case "MODE":
		fm, fok := parseFieldMode(value)
		if !fok {
			log.Printf("kutta: UDP control: MODE wants speed, vorticity, or pressure, got %q", value)
			return
		}
		g.enqueue(func() { g.mode = fm })
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
