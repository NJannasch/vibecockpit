package web

import "testing"

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int // -1, 0, +1 (sign only)
	}{
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.1.0", 1},
		{"v0.1.0", "v0.1.0", 0},
		{"0.1.0", "v0.1.0", 0},
		{"v0.1.0", "v0.10.0", -1},
		{"v1.0.0", "v0.99.99", 1},
		{"v1.2", "v1.2.0", 0},
		{"dev", "v0.1.0", -1},
		{"v0.1.0", "dev", 1},
		{"dev", "dev", 0},
		{"", "v1.0.0", -1},
	}
	for _, tt := range tests {
		got := compareSemver(tt.a, tt.b)
		gotSign := 0
		switch {
		case got < 0:
			gotSign = -1
		case got > 0:
			gotSign = 1
		}
		if gotSign != tt.want {
			t.Errorf("compareSemver(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}
