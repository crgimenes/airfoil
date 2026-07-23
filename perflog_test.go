package main

import "testing"

func TestPercentile(t *testing.T) {
	samples := []float64{5, 1, 4, 2, 3}
	cases := []struct {
		q    int
		want float64
	}{
		{50, 3},
		{95, 4},
		{100, 5},
		{0, 1},
	}
	for _, tc := range cases {
		got := percentile(samples, tc.q)
		if got != tc.want {
			t.Errorf("percentile(q=%d) = %v, want %v", tc.q, got, tc.want)
		}
	}
	if percentile(nil, 50) != 0 {
		t.Error("percentile of no samples should be 0")
	}
}

// A disabled perfLog must not collect anything, since it sits on the frame's
// hot path.
func TestPerfLogDisabledCollectsNothing(t *testing.T) {
	var p perfLog
	t0 := p.now()
	if !t0.IsZero() {
		t.Error("disabled now() should return the zero time")
	}
	p.add(&p.upd, t0)
	if len(p.upd) != 0 {
		t.Error("disabled add() should not record samples")
	}
}
