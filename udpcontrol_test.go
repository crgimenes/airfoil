package main

import "testing"

func TestParseControlMessage(t *testing.T) {
	cases := []struct {
		line    string
		channel string
		value   string
		wantOK  bool
	}{
		{"AOA 12.5", "AOA", "12.5", true},
		{"spd 0.08", "SPD", "0.08", true},
		{"mode vorticity", "MODE", "vorticity", true},
		{"  aoa   4  ", "AOA", "4", true},
		{"AOA", "", "", false},
		{"AOA 12.5 extra", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		channel, value, ok := parseControlMessage(c.line)
		if ok != c.wantOK {
			t.Errorf("parseControlMessage(%q) ok = %v, want %v", c.line, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if channel != c.channel || value != c.value {
			t.Errorf("parseControlMessage(%q) = (%q, %q), want (%q, %q)", c.line, channel, value, c.channel, c.value)
		}
	}
}

func TestIsMulticastAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{":9000", false},
		{"192.168.1.50:9000", false},
		{"0.0.0.0:9000", false},
		{"224.0.0.1:9000", true},   // IPv4 multicast, low end of 224.0.0.0/4
		{"239.192.1.1:1234", true}, // IPv4 multicast, admin-scoped range (kutta's suggested default)
		{"[ff02::1]:9000", true},   // IPv6 multicast
		{"not-an-addr", false},
	}
	for _, c := range cases {
		got := isMulticastAddr(c.addr)
		if got != c.want {
			t.Errorf("isMulticastAddr(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

// TestApplyControlMessageTogglesAndMode exercises the channels that don't
// need a real solver (GLOW, STREAMLINES, MODE), applying the message and then
// draining the enqueued closure exactly like Update() does each frame.
func TestApplyControlMessageTogglesAndMode(t *testing.T) {
	g := &Game{}

	g.applyControlMessage("GLOW 1")
	g.drainPending()
	if !g.glow {
		t.Error("GLOW 1 should turn glow on")
	}

	g.applyControlMessage("GLOW 0")
	g.drainPending()
	if g.glow {
		t.Error("GLOW 0 should turn glow off")
	}

	g.applyControlMessage("STREAMLINES 1")
	g.drainPending()
	if !g.streamlines {
		t.Error("STREAMLINES 1 should turn streamlines on")
	}

	g.applyControlMessage("PARTICLES 0")
	g.drainPending()
	if g.showParticles {
		t.Error("PARTICLES 0 should turn particles off")
	}

	g.applyControlMessage("PARTICLES 1")
	g.drainPending()
	if !g.showParticles {
		t.Error("PARTICLES 1 should turn particles on")
	}

	g.applyControlMessage("LABEL 1")
	g.drainPending()
	if !g.showLabel {
		t.Error("LABEL 1 should turn the legend on")
	}
	g.applyControlMessage("LABEL 0")
	g.drainPending()
	if g.showLabel {
		t.Error("LABEL 0 should turn the legend off")
	}

	g.applyControlMessage("CTRL 15")
	g.drainPending()
	if g.controlDeg != 15 {
		t.Errorf("CTRL 15: controlDeg = %v, want 15", g.controlDeg)
	}

	// setControl clamps to +-controlLimit; confirm CTRL inherits that.
	g.applyControlMessage("CTRL 999")
	g.drainPending()
	if g.controlDeg != controlLimit {
		t.Errorf("CTRL 999: controlDeg = %v, want clamped to %v", g.controlDeg, controlLimit)
	}

	g.applyControlMessage("MODE pressure")
	g.drainPending()
	if g.mode != modePressure {
		t.Errorf("mode = %v, want modePressure", g.mode)
	}

	// A bad value for a channel should be logged and ignored, not panic or
	// leave a stale enqueued closure.
	g.applyControlMessage("MODE sideways")
	g.drainPending()
	if g.mode != modePressure {
		t.Errorf("invalid MODE value should be ignored; mode = %v, want unchanged modePressure", g.mode)
	}

	g.applyControlMessage("DEMO 45")
	g.drainPending()
	if g.demoIdleSec != 45 {
		t.Errorf("DEMO 45: demoIdleSec = %v, want 45", g.demoIdleSec)
	}
	g.demoActive = true // simulate an in-progress wander
	g.applyControlMessage("DEMO 0")
	g.drainPending()
	if g.demoIdleSec != 0 {
		t.Errorf("DEMO 0: demoIdleSec = %v, want 0", g.demoIdleSec)
	}
	if g.demoActive {
		t.Error("DEMO 0 should hand control back immediately, not freeze mid-wander")
	}
}
