package inspect

import "testing"

func TestMinorSkew(t *testing.T) {
	tests := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"1.36.6", "v1.36.6", 0, true},
		{"v1.34.1", "1.36.6", 2, true},
		{"1.37.0", "1.36.6", 1, true},
		{"1.30+", "1.36", 6, true},
		{"2.0.0", "1.36.6", 0, false},
		{"", "1.36.6", 0, false},
	}
	for _, tt := range tests {
		got, ok := MinorSkew(tt.a, tt.b)
		if got != tt.want || ok != tt.ok {
			t.Errorf("MinorSkew(%q, %q) = %d, %v; want %d, %v", tt.a, tt.b, got, ok, tt.want, tt.ok)
		}
	}
}
