package uid

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"aa:bb:cc:dd", "AABBCCDD"},
		{"  AA BB CC ", "AABBCC"},
		{"deadbeef", "DEADBEEF"},
		{"", ""},
		{"  ", ""},
		{"a1 b2:c3", "A1B2C3"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
