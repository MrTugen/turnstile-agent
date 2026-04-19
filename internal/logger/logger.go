// Package logger provides a structured JSON logger that emits the same
// {"ts","level","msg","meta":{...}} shape as turnstile-edge so both daemons
// produce identical log lines.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// New returns a *slog.Logger configured to emit JSON with the shared field
// names. Structured key/value pairs passed to Info/Warn/etc are nested under a
// `meta` group.
//
// Errors and warnings go to stderr; info and debug to stdout.
func New(level string) *slog.Logger {
	lvl := parseLevel(level)
	return slog.New(newSplitHandler(lvl))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// splitHandler routes records to one of two underlying JSONHandlers depending
// on the level. It also ensures every non-builtin attribute lands inside a
// `meta` group, matching the shape `{ts, level, msg, meta?}`.
type splitHandler struct {
	out    *slog.JSONHandler
	err    *slog.JSONHandler
	attrs  []slog.Attr
	groups []string
}

func newSplitHandler(level slog.Level) *splitHandler {
	opts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceAttr,
	}
	return &splitHandler{
		out: slog.NewJSONHandler(os.Stdout, opts),
		err: slog.NewJSONHandler(os.Stderr, opts),
	}
}

// replaceAttr renames slog's builtin keys to match the shared log shape:
//
//	time  -> ts
//	level -> info|warn|... lowercased
//	msg   -> unchanged
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		a.Key = "ts"
	case slog.LevelKey:
		a.Value = slog.StringValue(strings.ToLower(a.Value.String()))
	}
	return a
}

func (h *splitHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.out.Enabled(ctx, level)
}

func (h *splitHandler) Handle(ctx context.Context, r slog.Record) error {
	meta := make([]any, 0, r.NumAttrs()*2)
	r.Attrs(func(a slog.Attr) bool {
		meta = append(meta, a.Key, a.Value.Any())
		return true
	})

	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	if len(meta) > 0 {
		out.AddAttrs(slog.Group("meta", meta...))
	}

	if r.Level >= slog.LevelWarn {
		return h.err.Handle(ctx, out)
	}
	return h.out.Handle(ctx, out)
}

func (h *splitHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &splitHandler{
		out:    h.out.WithAttrs(attrs).(*slog.JSONHandler),
		err:    h.err.WithAttrs(attrs).(*slog.JSONHandler),
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
	}
}

func (h *splitHandler) WithGroup(name string) slog.Handler {
	return &splitHandler{
		out:    h.out.WithGroup(name).(*slog.JSONHandler),
		err:    h.err.WithGroup(name).(*slog.JSONHandler),
		attrs:  h.attrs,
		groups: append(h.groups, name),
	}
}

// Discard is a no-op logger useful for tests.
func Discard() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}
