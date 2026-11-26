package main

import "testing"

func TestCompareVersionsFourPart(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.18.1", "0.1.18", 1},
		{"0.1.18", "v0.1.18.1", -1},
		{"v0.1.18.0", "0.1.18", 0},
		{"v0.1.19", "v0.1.18.9", 1},
		{"v0.1.18.2", "v0.1.18.10", -1},
		{"v0.1.18", "v0.1.18", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
	if releaseTag.MatchString("v0.1.18.1.1") || releaseTag.MatchString("v0.1.x") {
		t.Error("invalid tags must not match")
	}
}
