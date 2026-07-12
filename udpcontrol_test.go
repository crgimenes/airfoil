package main

import "testing"

func TestParseControlMessage(t *testing.T) {
	cases := []struct {
		line    string
		channel string
		value   float64
		wantOK  bool
	}{
		{"AOA 12.5", "AOA", 12.5, true},
		{"spd 0.08", "SPD", 0.08, true},
		{"ctrl -20", "CTRL", -20, true},
		{"  aoa   4  ", "AOA", 4, true},
		{"AOA", "", 0, false},
		{"AOA 12.5 extra", "", 0, false},
		{"AOA notanumber", "", 0, false},
		{"", "", 0, false},
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
			t.Errorf("parseControlMessage(%q) = (%q, %v), want (%q, %v)", c.line, channel, value, c.channel, c.value)
		}
	}
}
