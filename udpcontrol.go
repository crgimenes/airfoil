package main

import (
	"bufio"
	"log"
	"net"
	"strconv"
	"strings"
)

// startUDPControl opens a UDP listener at addr (e.g. ":9000") and applies
// incoming slider commands as they arrive, for driving kutta from external
// hardware -- e.g. an ESP32 with rotary encoders. UDP is the only one of the
// common options (WebSockets, UDP, MQTT) that needs no new dependency and no
// broker process, matching this project's single-executable, family-stack
// rule. Nothing is sent back; this is receive-only.
//
// Currently wired to angle of attack (AOA) and inlet speed (SPD); a
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
// matching slider update to run on the game goroutine -- the UDP loop is its
// own goroutine, so game state can't be touched from it directly.
func (g *Game) applyControlMessage(line string) {
	cmd, value, ok := parseControlMessage(line)
	if !ok {
		log.Printf("kutta: UDP control: bad message %q", line)
		return
	}
	switch cmd {
	case "AOA":
		g.enqueue(func() { g.setAlpha(value) })
	case "SPD":
		g.enqueue(func() { g.setSpeed(value) })
	default:
		log.Printf("kutta: UDP control: unknown channel %q", cmd)
	}
}

// parseControlMessage parses one "CHANNEL VALUE" line (whitespace-separated,
// case-insensitive channel), e.g. "AOA 12.5". It is a pure function so the
// wire protocol can be tested headlessly, without a socket.
func parseControlMessage(line string) (channel string, value float64, ok bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return "", 0, false
	}
	v, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return "", 0, false
	}
	return strings.ToUpper(fields[0]), v, true
}
