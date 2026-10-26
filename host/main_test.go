package main

import "testing"

func TestCommandBoundaries(t *testing.T) {
	for _, cmd := range []string{"status", "face nunito", "face-get", "face-set digit0 10,10,55,88", "face-reset", "profile bedside", "brightness 1", "brightness 100", "offset -720", "offset 840", "timer 0", "timer 86400", "alarm off", "alarm 23 59", "stats 0 100", "event double", "time 1800000000"} {
		if e := validateCommand(cmd); e != nil {
			t.Fatalf("%q: %v", cmd, e)
		}
	}
	for _, cmd := range []string{"", "face ../bad", "face-set atlas ../../bad", "face-set color $(id)", "status ; reboot", "profile minimal;reboot", "brightness 0", "brightness 101", "offset -721", "timer -1", "alarm 24 0", "alarm 1 60", "stats 1", "event reboot", "time 0", "unknown", "profile $(id)"} {
		if e := validateCommand(cmd); e == nil {
			t.Fatalf("unsafe/invalid command accepted: %q", cmd)
		}
	}
}
