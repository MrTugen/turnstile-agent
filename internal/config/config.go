// Package config loads the turnstile-agent runtime configuration from
// environment variables (with optional .env support). Variable names and
// defaults match the previous Python implementation so existing operator .env
// files keep working unchanged.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	EdgeURL         string
	EdgeAPIKey      string
	DeviceName      string
	GPIOChip        string
	GPIOPin         int
	RelayActiveHigh bool
	PulseMs         int
	ScanCooldownMs  int
	RequestTimeout  time.Duration

	ReaderEventPath string
	ReaderName      string
	ReaderPhys      string

	OfflineAllowEnabled  bool
	OfflineAllowlistPath string

	LogLevel string
}

// Load resolves the configuration from the process environment, after reading
// `.env` (if present) without overriding already-set variables.
func Load() (*Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return nil, fmt.Errorf("read .env: %w", err)
	}
	return parse()
}

func parse() (*Config, error) {
	gpio, err := intEnv("GPIO_PIN", 17)
	if err != nil {
		return nil, err
	}
	pulseMs, err := intEnv("PULSE_MS", 300)
	if err != nil {
		return nil, err
	}
	cooldown, err := intEnv("SCAN_COOLDOWN_MS", 2000)
	if err != nil {
		return nil, err
	}
	timeout, err := floatEnv("REQUEST_TIMEOUT_SEC", 3.0)
	if err != nil {
		return nil, err
	}

	return &Config{
		EdgeURL:         envOr("EDGE_URL", "http://127.0.0.1:3000/api/turnstile/scan"),
		EdgeAPIKey:      envOr("EDGE_API_KEY", ""),
		DeviceName:      envOr("DEVICE_NAME", "turnstile-pi-01"),
		GPIOChip:        envOr("GPIO_CHIP", "gpiochip0"),
		GPIOPin:         gpio,
		RelayActiveHigh: boolEnv("RELAY_ACTIVE_HIGH", true),
		PulseMs:         pulseMs,
		ScanCooldownMs:  cooldown,
		RequestTimeout:  time.Duration(timeout * float64(time.Second)),

		ReaderEventPath: envOr("READER_EVENT_PATH", ""),
		ReaderName:      envOr("READER_NAME", "ACS ACR1281 Dual Reader"),
		ReaderPhys:      envOr("READER_PHYS", ""),

		OfflineAllowEnabled:  boolEnv("OFFLINE_ALLOW_ENABLED", false),
		OfflineAllowlistPath: envOr("OFFLINE_ALLOWLIST_PATH", "/opt/turnstile-agent/allowlist.json"),

		LogLevel: strings.ToLower(envOr("LOG_LEVEL", "INFO")),
	}, nil
}

func envOr(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return fallback
}

func intEnv(name string, fallback int) (int, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %s", name, v)
	}
	return parsed, nil
}

func floatEnv(name string, fallback float64) (float64, error) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float for %s: %s", name, v)
	}
	return parsed, nil
}

func boolEnv(name string, fallback bool) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
