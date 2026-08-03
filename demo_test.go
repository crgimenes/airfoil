package main

import (
	"testing"
	"time"
)

// TestDemoModeIdleAndCancel pins the -demo state machine: it must not
// activate before demoIdleSec of no real input has elapsed, and any real
// input (via the same setSpeed/setAlpha/setControl choke points sliders,
// keyboard and UDP all go through) must cancel it immediately.
func TestDemoModeIdleAndCancel(t *testing.T) {
	g := simGame()
	g.demoIdleSec = 60

	g.lastUserInput = time.Now()
	g.updateDemo()
	if g.demoActive {
		t.Error("demo should not activate before the idle interval elapses")
	}

	g.lastUserInput = time.Now().Add(-61 * time.Second)
	g.updateDemo()
	if !g.demoActive {
		t.Error("demo should activate once idle for longer than demoIdleSec")
	}

	g.setAlpha(g.alphaDeg + 1)
	if g.demoActive {
		t.Error("real input (setAlpha) should cancel demo mode immediately")
	}
}

// TestDemoModeDisabledByDefault pins demoIdleSec <= 0 as "feature off": no
// amount of idle time should ever activate it.
func TestDemoModeDisabledByDefault(t *testing.T) {
	g := simGame()
	g.demoIdleSec = 0
	g.lastUserInput = time.Now().Add(-time.Hour)
	g.updateDemo()
	if g.demoActive {
		t.Error("demoIdleSec <= 0 should never activate demo mode")
	}
}

// TestDemoModeDriverDoesNotSelfCancel pins the applyingDemo guard: the demo
// driver's own calls to the setters must not reset lastUserInput or cancel
// demoActive, or the feature could never sustain a wander.
func TestDemoModeDriverDoesNotSelfCancel(t *testing.T) {
	g := simGame()
	g.demoIdleSec = 60
	g.demoActive = true
	g.demoTargetSpeed = g.u0 + 0.01
	g.demoTargetAoa = g.alphaDeg + 5
	g.demoTargetCtrl = 10
	g.demoNextRetime = time.Now().Add(time.Hour) // don't retarget mid-test

	g.updateDemo()
	if !g.demoActive {
		t.Error("the demo driver's own updates must not cancel demoActive")
	}
}
