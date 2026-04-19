package config

import (
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	// Clear all relevant env vars so defaults apply.
	for _, k := range []string{
		"EDGE_URL", "EDGE_API_KEY", "DEVICE_NAME",
		"GPIO_CHIP", "GPIO_PIN", "RELAY_ACTIVE_HIGH",
		"PULSE_MS", "SCAN_COOLDOWN_MS", "REQUEST_TIMEOUT_SEC",
		"READER_EVENT_PATH", "READER_NAME", "READER_PHYS",
		"OFFLINE_ALLOW_ENABLED", "OFFLINE_ALLOWLIST_PATH", "LOG_LEVEL",
	} {
		t.Setenv(k, "")
	}

	cfg, err := parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Spot-check a handful of defaults that must match the Python agent.
	if cfg.GPIOPin != 17 {
		t.Errorf("GPIOPin = %d, want 17", cfg.GPIOPin)
	}
	if cfg.PulseMs != 300 {
		t.Errorf("PulseMs = %d, want 300", cfg.PulseMs)
	}
	if cfg.ScanCooldownMs != 2000 {
		t.Errorf("ScanCooldownMs = %d, want 2000", cfg.ScanCooldownMs)
	}
	if cfg.RequestTimeout != 3*time.Second {
		t.Errorf("RequestTimeout = %s, want 3s", cfg.RequestTimeout)
	}
	if !cfg.RelayActiveHigh {
		t.Error("RelayActiveHigh = false, want true")
	}
	if cfg.OfflineAllowEnabled {
		t.Error("OfflineAllowEnabled = true, want false")
	}
	if cfg.DeviceName != "turnstile-pi-01" {
		t.Errorf("DeviceName = %q, want turnstile-pi-01", cfg.DeviceName)
	}
	if cfg.ReaderName != "ACS ACR1281 Dual Reader" {
		t.Errorf("ReaderName = %q, want ACS ACR1281 Dual Reader", cfg.ReaderName)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestParseOverrides(t *testing.T) {
	t.Setenv("GPIO_PIN", "22")
	t.Setenv("PULSE_MS", "500")
	t.Setenv("REQUEST_TIMEOUT_SEC", "1.5")
	t.Setenv("RELAY_ACTIVE_HIGH", "false")
	t.Setenv("OFFLINE_ALLOW_ENABLED", "true")
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg, err := parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cfg.GPIOPin != 22 {
		t.Errorf("GPIOPin = %d, want 22", cfg.GPIOPin)
	}
	if cfg.PulseMs != 500 {
		t.Errorf("PulseMs = %d, want 500", cfg.PulseMs)
	}
	if cfg.RequestTimeout != 1500*time.Millisecond {
		t.Errorf("RequestTimeout = %s, want 1.5s", cfg.RequestTimeout)
	}
	if cfg.RelayActiveHigh {
		t.Error("RelayActiveHigh = true, want false")
	}
	if !cfg.OfflineAllowEnabled {
		t.Error("OfflineAllowEnabled = false, want true")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestParseInvalidInt(t *testing.T) {
	t.Setenv("GPIO_PIN", "not-a-number")
	if _, err := parse(); err == nil {
		t.Fatal("expected error for invalid GPIO_PIN, got nil")
	}
}
