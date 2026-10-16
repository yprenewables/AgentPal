package update

import "testing"

func TestCompareVersionHandlesPrereleaseSuffixes(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.11-beta", "0.1.2", 1},
		{"0.11-beta", "0.11-beta", 0},
		{"v", "", 0},
		{"0.2.0", "0.11-beta", -1},
		{"0.11.1", "0.11-beta", 1},
	} {
		if got := compareVersion(tc.a, tc.b); got != tc.want {
			t.Fatalf("compareVersion(%q, %q)=%d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
