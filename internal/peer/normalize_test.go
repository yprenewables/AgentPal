package peer

import "testing"

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"192.168.1.23":                       "http://192.168.1.23:28888",
		"192.168.1.23:28889":                 "http://192.168.1.23:28889",
		"http://192.168.1.23:28888":          "http://192.168.1.23:28888",
		"http://192.168.1.23:28888/manifest": "http://192.168.1.23:28888",
	}
	for input, want := range tests {
		got, err := Normalize(input, 28888)
		if err != nil {
			t.Fatalf("Normalize(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeRejectsInvalidInputs(t *testing.T) {
	for _, input := range []string{"", "https://192.168.1.23:28888", "ftp://192.168.1.23"} {
		if _, err := Normalize(input, 28888); err == nil {
			t.Fatalf("Normalize(%q) expected an error", input)
		}
	}
}
