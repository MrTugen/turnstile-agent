// Package uid holds the shared RFID UID normalization used by the edge client,
// allowlist, and agent dedupe logic.
package uid

import "strings"

// Normalize returns the canonical form of a UID: stripped of whitespace and
// colons, upper-cased. Matches the Python agent's normalization rule so the
// same operator allowlists keep working.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.ToUpper(s)
}
