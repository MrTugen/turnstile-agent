//go:build !linux

package relay

import (
	"log/slog"
	"time"
)

// Relay is a no-op stub on non-Linux platforms; Pulse just logs and sleeps.
type Relay struct {
	pin int
	log *slog.Logger
}

// Open logs a warning and returns a stub that simulates pulses.
func Open(_ string, pin int, _ bool, log *slog.Logger) (*Relay, error) {
	log.Warn("GPIO relay is not supported on this platform; simulating pulses", "pin", pin)
	return &Relay{pin: pin, log: log}, nil
}

// Pulse logs and sleeps for the configured duration.
func (r *Relay) Pulse(d time.Duration) {
	r.log.Info("Triggering relay (simulated)", "pin", r.pin, "duration", d.String())
	time.Sleep(d)
}

// Close is a no-op.
func (r *Relay) Close() error { return nil }
