// Package agent orchestrates a scan: dedupe → edge verify → relay pulse, with
// an offline-allowlist fallback when the edge is unreachable.
package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/MrTugen/turnstile-agent/internal/allowlist"
	"github.com/MrTugen/turnstile-agent/internal/edge"
	"github.com/MrTugen/turnstile-agent/internal/uid"
)

// EdgeClient is the subset of edge.Client the agent needs; interface makes it
// trivial to substitute a fake in tests.
type EdgeClient interface {
	VerifyScan(ctx context.Context, uid string) (edge.Decision, error)
}

// Relay is the subset of relay.Relay the agent needs.
type Relay interface {
	Pulse(d time.Duration)
}

// Options wire dependencies into an Agent.
type Options struct {
	Edge           EdgeClient
	Relay          Relay
	Allowlist      *allowlist.Allowlist
	Log            *slog.Logger
	PulseDuration  time.Duration
	ScanCooldown   time.Duration
	RequestTimeout time.Duration
}

// Agent handles a single scan at a time, with per-UID cooldown.
type Agent struct {
	opts Options

	mu         sync.Mutex
	lastUID    string
	lastScanAt time.Time
}

// New constructs an Agent with the given dependencies.
func New(opts Options) *Agent {
	return &Agent{opts: opts}
}

// HandleScan runs the full decision pipeline for one UID.
// Safe to call from the reader goroutine.
func (a *Agent) HandleScan(ctx context.Context, rawUID string) {
	u := uid.Normalize(rawUID)
	if u == "" {
		return
	}

	if a.shouldIgnore(u) {
		a.opts.Log.Info("Ignoring duplicate scan", "uid", u)
		return
	}

	a.opts.Log.Info("Received UID", "uid", u)

	granted := false
	reason := "unknown"

	reqCtx, cancel := context.WithTimeout(ctx, a.opts.RequestTimeout)
	defer cancel()

	decision, err := a.opts.Edge.VerifyScan(reqCtx, u)
	if err != nil {
		a.opts.Log.Error("Edge verification failed", "uid", u, "error", err.Error())
		if a.opts.Allowlist.IsAllowed(u) {
			granted = true
			reason = "offline_allowlist"
		} else {
			granted = false
			reason = "edge_error"
		}
	} else {
		granted = decision.Granted
		reason = decision.Reason
	}

	if granted {
		a.opts.Log.Info("Access granted", "uid", u, "reason", reason)
		a.opts.Relay.Pulse(a.opts.PulseDuration)
	} else {
		a.opts.Log.Warn("Access denied", "uid", u, "reason", reason)
	}
}

// shouldIgnore returns true when the same UID was scanned within the cooldown
// window. Updates the last-scan state as a side-effect even when returning
// false so the cooldown starts from the most recent scan.
func (a *Agent) shouldIgnore(u string) bool {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.lastUID == u && now.Sub(a.lastScanAt) < a.opts.ScanCooldown {
		return true
	}
	a.lastUID = u
	a.lastScanAt = now
	return false
}
