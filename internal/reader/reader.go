//go:build linux

// Package reader reads RFID UIDs from an HID keyboard-emulating card reader
// exposed as a Linux evdev input device.
package reader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/holoplot/go-evdev"
)

// ResolveConfig holds the subset of turnstile-agent config needed to locate
// the right /dev/input/eventN device.
type ResolveConfig struct {
	EventPath string // if set, used directly
	Name      string // exact match on device name (empty = any)
	Phys      string // exact match on device phys (empty = any)
}

// Resolve returns the /dev/input/eventN path for the configured card reader.
// It mirrors the Python agent's semantics: explicit path wins; otherwise filter
// all input devices by Name/Phys and require exactly one match.
func Resolve(cfg ResolveConfig) (string, error) {
	if cfg.EventPath != "" {
		if _, err := os.Stat(cfg.EventPath); err != nil {
			return "", fmt.Errorf("configured reader path does not exist: %s", cfg.EventPath)
		}
		return cfg.EventPath, nil
	}

	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return "", fmt.Errorf("list input devices: %w", err)
	}

	type candidate struct {
		path, name, phys string
	}

	var matches []candidate
	var allDevices []candidate

	for _, p := range paths {
		dev, err := evdev.Open(p.Path)
		if err != nil {
			continue
		}
		name, _ := dev.Name()
		phys, _ := dev.PhysicalLocation()
		dev.Close()

		allDevices = append(allDevices, candidate{p.Path, name, phys})

		if cfg.Name != "" && name != cfg.Name {
			continue
		}
		if cfg.Phys != "" && phys != cfg.Phys {
			continue
		}
		matches = append(matches, candidate{p.Path, name, phys})
	}

	if len(matches) == 0 {
		available := "none"
		if len(allDevices) > 0 {
			parts := make([]string, 0, len(allDevices))
			for _, d := range allDevices {
				parts = append(parts, fmt.Sprintf("%s (%s)", d.path, d.name))
			}
			available = strings.Join(parts, ", ")
		}
		return "", fmt.Errorf(
			"no matching reader found for name=%q phys=%q. Available devices: %s",
			cfg.Name, cfg.Phys, available,
		)
	}

	if len(matches) > 1 {
		parts := make([]string, 0, len(matches))
		for _, d := range matches {
			parts = append(parts, fmt.Sprintf("%s (%s, phys=%s)", d.path, d.name, d.phys))
		}
		return "", fmt.Errorf(
			"multiple matching readers found; set READER_EVENT_PATH or READER_PHYS to disambiguate: %s",
			strings.Join(parts, ", "),
		)
	}

	return matches[0].path, nil
}

// keyMap maps HID key codes emitted by the reader to UID characters. The
// reader is a 10-digit numeric keypad plus A-F for hex UIDs.
var keyMap = map[evdev.EvCode]byte{
	evdev.KEY_0: '0', evdev.KEY_1: '1', evdev.KEY_2: '2', evdev.KEY_3: '3', evdev.KEY_4: '4',
	evdev.KEY_5: '5', evdev.KEY_6: '6', evdev.KEY_7: '7', evdev.KEY_8: '8', evdev.KEY_9: '9',
	evdev.KEY_A: 'A', evdev.KEY_B: 'B', evdev.KEY_C: 'C', evdev.KEY_D: 'D', evdev.KEY_E: 'E',
	evdev.KEY_F: 'F',
}

// Reader streams UIDs from an HID keyboard-emulating card reader.
type Reader struct {
	path string
	dev  *evdev.InputDevice
	log  *slog.Logger
}

// Open opens the evdev device at the given path.
func Open(path string, log *slog.Logger) (*Reader, error) {
	dev, err := evdev.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open evdev %s: %w", path, err)
	}
	return &Reader{path: path, dev: dev, log: log}, nil
}

// Close releases the underlying device.
func (r *Reader) Close() error {
	if r.dev != nil {
		return r.dev.Close()
	}
	return nil
}

// ReadLoop reads key events and invokes onUID for each completed UID
// (characters accumulated between key presses, flushed on ENTER).
// It returns when ctx is cancelled or an unrecoverable read error occurs.
func (r *Reader) ReadLoop(ctx context.Context, onUID func(string)) error {
	name, _ := r.dev.Name()
	r.log.Info("Listening for card scans", "path", r.path, "name", name)

	// Close the device from a background goroutine when ctx is cancelled so
	// that the blocking ReadOne unblocks with an error.
	go func() {
		<-ctx.Done()
		_ = r.dev.Close()
	}()

	var buf []byte
	for {
		event, err := r.dev.ReadOne()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, os.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read evdev: %w", err)
		}
		if event.Type != evdev.EV_KEY {
			continue
		}
		// Value: 0=release, 1=press, 2=repeat. Only act on press.
		if event.Value != 1 {
			continue
		}

		if event.Code == evdev.KEY_ENTER || event.Code == evdev.KEY_KPENTER {
			if len(buf) > 0 {
				uid := string(buf)
				buf = buf[:0]
				onUID(uid)
			}
			continue
		}

		if ch, ok := keyMap[event.Code]; ok {
			buf = append(buf, ch)
		}
	}
}
