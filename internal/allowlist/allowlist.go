// Package allowlist loads an offline JSON allowlist of RFID UIDs that should
// be admitted when the edge is unreachable.
package allowlist

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"

	"github.com/MrTugen/turnstile-agent/internal/uid"
)

// Allowlist is a set of normalized UIDs loaded from a JSON array file.
type Allowlist struct {
	enabled bool
	allowed map[string]struct{}
}

// Load reads the allowlist file if enabled. Missing file and malformed JSON
// are logged at warn level but never fatal — the Python agent's behavior.
func Load(enabled bool, path string, log *slog.Logger) *Allowlist {
	a := &Allowlist{
		enabled: enabled,
		allowed: make(map[string]struct{}),
	}
	if !enabled {
		return a
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.Warn("Offline allowlist file not found", "path", path)
		} else {
			log.Warn("Failed to read offline allowlist", "path", path, "error", err.Error())
		}
		return a
	}

	var raw []string
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Warn("Offline allowlist is not a JSON array of strings", "path", path, "error", err.Error())
		return a
	}

	for _, s := range raw {
		a.allowed[uid.Normalize(s)] = struct{}{}
	}
	log.Info("Loaded offline allowlist", "count", len(a.allowed))
	return a
}

// IsAllowed returns true if the UID is present in the allowlist and the
// allowlist is enabled.
func (a *Allowlist) IsAllowed(rawUID string) bool {
	if !a.enabled {
		return false
	}
	_, ok := a.allowed[uid.Normalize(rawUID)]
	return ok
}

// Count returns the number of UIDs loaded (for logging/tests).
func (a *Allowlist) Count() int { return len(a.allowed) }
