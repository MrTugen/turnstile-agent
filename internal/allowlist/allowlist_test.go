package allowlist

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MrTugen/turnstile-agent/internal/logger"
)

func TestDisabledAlwaysDenies(t *testing.T) {
	a := Load(false, "/nonexistent/path", logger.Discard())
	if a.IsAllowed("AABBCC") {
		t.Error("IsAllowed returned true for disabled allowlist")
	}
}

func TestMissingFileLoadsEmpty(t *testing.T) {
	a := Load(true, filepath.Join(t.TempDir(), "missing.json"), logger.Discard())
	if a.Count() != 0 {
		t.Errorf("Count = %d, want 0", a.Count())
	}
	if a.IsAllowed("AABBCC") {
		t.Error("IsAllowed returned true for missing-file allowlist")
	}
}

func TestLoadAndNormalize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.json")
	body := `[" aa:bb:cc ", "DeadBeef", "ff ff ff"]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	a := Load(true, path, logger.Discard())
	if a.Count() != 3 {
		t.Errorf("Count = %d, want 3", a.Count())
	}
	cases := []struct {
		in   string
		want bool
	}{
		{"AABBCC", true},
		{"aa:bb:cc", true},
		{"deadbeef", true},
		{"FFFFFF", true},
		{"112233", false},
	}
	for _, c := range cases {
		if got := a.IsAllowed(c.in); got != c.want {
			t.Errorf("IsAllowed(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMalformedJSONLoadsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.json")
	if err := os.WriteFile(path, []byte(`{"not":"an array"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a := Load(true, path, logger.Discard())
	if a.Count() != 0 {
		t.Errorf("Count = %d, want 0 (malformed JSON)", a.Count())
	}
}
