// Command turnstile-agent runs on a Raspberry Pi inside each turnstile. It
// reads RFID UIDs from an HID card reader, posts them to turnstile-edge for
// access verification, and pulses a GPIO relay when access is granted.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MrTugen/turnstile-agent/internal/agent"
	"github.com/MrTugen/turnstile-agent/internal/allowlist"
	"github.com/MrTugen/turnstile-agent/internal/config"
	"github.com/MrTugen/turnstile-agent/internal/edge"
	"github.com/MrTugen/turnstile-agent/internal/logger"
	"github.com/MrTugen/turnstile-agent/internal/reader"
	"github.com/MrTugen/turnstile-agent/internal/relay"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		os.Stderr.WriteString("turnstile-agent: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info("Starting Turnstile Agent",
		"deviceName", cfg.DeviceName,
		"edgeURL", cfg.EdgeURL,
		"gpioChip", cfg.GPIOChip,
		"gpioPin", cfg.GPIOPin,
		"readerEventPath", orAuto(cfg.ReaderEventPath),
		"readerName", orNone(cfg.ReaderName),
		"readerPhys", orNone(cfg.ReaderPhys),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- core wiring ---
	rly, err := relay.Open(cfg.GPIOChip, cfg.GPIOPin, cfg.RelayActiveHigh, log)
	if err != nil {
		log.Error("Failed to open relay", "error", err.Error())
		os.Exit(1)
	}
	defer func() { _ = rly.Close() }()

	allow := allowlist.Load(cfg.OfflineAllowEnabled, cfg.OfflineAllowlistPath, log)

	edgeClient := edge.New(edge.Options{
		URL:            cfg.EdgeURL,
		APIKey:         cfg.EdgeAPIKey,
		DeviceName:     cfg.DeviceName,
		RequestTimeout: cfg.RequestTimeout,
	})

	ag := agent.New(agent.Options{
		Edge:           edgeClient,
		Relay:          rly,
		Allowlist:      allow,
		Log:            log,
		PulseDuration:  time.Duration(cfg.PulseMs) * time.Millisecond,
		ScanCooldown:   time.Duration(cfg.ScanCooldownMs) * time.Millisecond,
		RequestTimeout: cfg.RequestTimeout,
	})

	// --- reader ---
	readerPath, err := reader.Resolve(reader.ResolveConfig{
		EventPath: cfg.ReaderEventPath,
		Name:      cfg.ReaderName,
		Phys:      cfg.ReaderPhys,
	})
	if err != nil {
		log.Error("Failed to resolve reader", "error", err.Error())
		os.Exit(1)
	}
	log.Info("Resolved reader path", "path", readerPath)

	rdr, err := reader.Open(readerPath, log)
	if err != nil {
		log.Error("Failed to open reader", "error", err.Error())
		os.Exit(1)
	}
	defer func() { _ = rdr.Close() }()

	// Reader loop runs until ctx is cancelled (signal handler closes the device).
	readerErr := make(chan error, 1)
	go func() {
		readerErr <- rdr.ReadLoop(ctx, func(uid string) {
			ag.HandleScan(ctx, uid)
		})
	}()

	select {
	case <-ctx.Done():
		log.Warn("Shutting down")
	case err := <-readerErr:
		if err != nil {
			log.Error("Reader loop exited with error", "error", err.Error())
		}
	}

	log.Info("turnstile-agent stopped")
}

func orAuto(s string) string {
	if s == "" {
		return "<auto>"
	}
	return s
}

func orNone(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}
