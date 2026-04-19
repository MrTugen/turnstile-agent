//go:build linux

// Package relay drives the GPIO line that pulses the turnstile's relay.
package relay

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

// Relay owns a single GPIO output line.
type Relay struct {
	chip       string
	pin        int
	activeHigh bool
	line       *gpiocdev.Line
	log        *slog.Logger
}

// Open requests the configured GPIO line as an output driven to its inactive
// state.
func Open(chip string, pin int, activeHigh bool, log *slog.Logger) (*Relay, error) {
	inactive := 0
	if !activeHigh {
		inactive = 1
	}

	line, err := gpiocdev.RequestLine(chip, pin, gpiocdev.AsOutput(inactive), gpiocdev.WithConsumer("turnstile-agent"))
	if err != nil {
		return nil, fmt.Errorf("request gpio line %s/%d: %w", chip, pin, err)
	}

	log.Info("Relay ready", "chip", chip, "pin", pin, "activeHigh", activeHigh)
	return &Relay{
		chip:       chip,
		pin:        pin,
		activeHigh: activeHigh,
		line:       line,
		log:        log,
	}, nil
}

// Pulse drives the relay active for the given duration, then inactive.
func (r *Relay) Pulse(d time.Duration) {
	active, inactive := 1, 0
	if !r.activeHigh {
		active, inactive = 0, 1
	}

	r.log.Info("Triggering relay", "pin", r.pin, "duration", d.String())

	if err := r.line.SetValue(active); err != nil {
		r.log.Error("failed to activate relay", "error", err.Error())
		return
	}
	time.Sleep(d)
	if err := r.line.SetValue(inactive); err != nil {
		r.log.Error("failed to deactivate relay", "error", err.Error())
	}
}

// Close drives the line inactive and releases it.
func (r *Relay) Close() error {
	inactive := 0
	if !r.activeHigh {
		inactive = 1
	}
	_ = r.line.SetValue(inactive)
	return r.line.Close()
}
