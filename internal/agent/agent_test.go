package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MrTugen/turnstile-agent/internal/allowlist"
	"github.com/MrTugen/turnstile-agent/internal/edge"
	"github.com/MrTugen/turnstile-agent/internal/logger"
)

type fakeEdge struct {
	calls    atomic.Int32
	decision edge.Decision
	err      error
}

func (f *fakeEdge) VerifyScan(ctx context.Context, uid string) (edge.Decision, error) {
	f.calls.Add(1)
	return f.decision, f.err
}

type fakeRelay struct {
	pulses atomic.Int32
}

func (r *fakeRelay) Pulse(d time.Duration) { r.pulses.Add(1) }

func makeAgent(t *testing.T, e *fakeEdge, r *fakeRelay, a *allowlist.Allowlist, cooldown time.Duration) *Agent {
	t.Helper()
	return New(Options{
		Edge:           e,
		Relay:          r,
		Allowlist:      a,
		Log:            logger.Discard(),
		PulseDuration:  10 * time.Millisecond,
		ScanCooldown:   cooldown,
		RequestTimeout: time.Second,
	})
}

func TestHandleScanGrantedPulsesRelay(t *testing.T) {
	e := &fakeEdge{decision: edge.Decision{Granted: true, Reason: "ok"}}
	r := &fakeRelay{}
	ag := makeAgent(t, e, r, allowlist.Load(false, "", logger.Discard()), time.Second)

	ag.HandleScan(context.Background(), "aa:bb")

	if got := r.pulses.Load(); got != 1 {
		t.Errorf("pulses = %d, want 1", got)
	}
}

func TestHandleScanDeniedDoesNotPulse(t *testing.T) {
	e := &fakeEdge{decision: edge.Decision{Granted: false, Reason: "no"}}
	r := &fakeRelay{}
	ag := makeAgent(t, e, r, allowlist.Load(false, "", logger.Discard()), time.Second)

	ag.HandleScan(context.Background(), "AA")

	if got := r.pulses.Load(); got != 0 {
		t.Errorf("pulses = %d, want 0", got)
	}
}

func TestHandleScanDedupesWithinCooldown(t *testing.T) {
	e := &fakeEdge{decision: edge.Decision{Granted: true}}
	r := &fakeRelay{}
	ag := makeAgent(t, e, r, allowlist.Load(false, "", logger.Discard()), time.Second)

	ag.HandleScan(context.Background(), "AA")
	ag.HandleScan(context.Background(), "AA") // dupe within cooldown

	if got := e.calls.Load(); got != 1 {
		t.Errorf("edge calls = %d, want 1", got)
	}
	if got := r.pulses.Load(); got != 1 {
		t.Errorf("pulses = %d, want 1", got)
	}
}

func TestHandleScanDifferentUIDBypassesCooldown(t *testing.T) {
	e := &fakeEdge{decision: edge.Decision{Granted: true}}
	r := &fakeRelay{}
	ag := makeAgent(t, e, r, allowlist.Load(false, "", logger.Discard()), time.Second)

	ag.HandleScan(context.Background(), "AA")
	ag.HandleScan(context.Background(), "BB")

	if got := e.calls.Load(); got != 2 {
		t.Errorf("edge calls = %d, want 2", got)
	}
}

func TestHandleScanOfflineAllowlistFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allow.json")
	if err := os.WriteFile(path, []byte(`["AABB"]`), 0o600); err != nil {
		t.Fatal(err)
	}

	e := &fakeEdge{err: errors.New("network down")}
	r := &fakeRelay{}
	allow := allowlist.Load(true, path, logger.Discard())
	ag := makeAgent(t, e, r, allow, time.Second)

	ag.HandleScan(context.Background(), "aa:bb")

	if got := r.pulses.Load(); got != 1 {
		t.Errorf("pulses = %d, want 1 (allowlist should have granted)", got)
	}
}

func TestHandleScanEdgeErrorDeniesWithoutAllowlist(t *testing.T) {
	e := &fakeEdge{err: errors.New("network down")}
	r := &fakeRelay{}
	ag := makeAgent(t, e, r, allowlist.Load(false, "", logger.Discard()), time.Second)

	ag.HandleScan(context.Background(), "AA")

	if got := r.pulses.Load(); got != 0 {
		t.Errorf("pulses = %d, want 0", got)
	}
}

func TestHandleScanEmptyUIDDropped(t *testing.T) {
	e := &fakeEdge{}
	r := &fakeRelay{}
	ag := makeAgent(t, e, r, allowlist.Load(false, "", logger.Discard()), time.Second)

	ag.HandleScan(context.Background(), "   ")

	if got := e.calls.Load(); got != 0 {
		t.Errorf("edge calls = %d, want 0", got)
	}
}
