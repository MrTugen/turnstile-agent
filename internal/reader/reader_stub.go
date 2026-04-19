//go:build !linux

package reader

import (
	"context"
	"errors"
	"log/slog"
)

// ResolveConfig is a cross-platform-compatible echo of the Linux variant.
type ResolveConfig struct {
	EventPath string
	Name      string
	Phys      string
}

// Resolve is not supported outside Linux. Returns an error so the agent fails
// fast if run on a non-Pi host.
func Resolve(_ ResolveConfig) (string, error) {
	return "", errors.New("evdev card reader is only supported on Linux")
}

// Reader is a non-functional stub on non-Linux platforms.
type Reader struct {
	log *slog.Logger
}

// Open always returns an error on non-Linux platforms.
func Open(_ string, log *slog.Logger) (*Reader, error) {
	return nil, errors.New("evdev card reader is only supported on Linux")
}

// Close is a no-op.
func (r *Reader) Close() error { return nil }

// ReadLoop is a no-op that blocks until ctx is cancelled.
func (r *Reader) ReadLoop(ctx context.Context, _ func(string)) error {
	<-ctx.Done()
	return nil
}
