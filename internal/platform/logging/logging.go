// Package logging configures structured logs that Cloud Logging understands
// without an agent: JSON on stdout with "severity" and "message" keys, plus
// the request ID and trace of the request that produced each line.
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	traceKey
)

// WithRequestID stores the request ID for log correlation and error payloads.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the ID stored by WithRequestID.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WithTrace stores a Cloud Trace resource name
// ("projects/<project>/traces/<trace-id>").
func WithTrace(ctx context.Context, trace string) context.Context {
	return context.WithValue(ctx, traceKey, trace)
}

// New returns a JSON logger. format "text" is friendlier in a terminal.
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: cloudLoggingKeys}
	var h slog.Handler = slog.NewJSONHandler(w, opts)
	if strings.EqualFold(format, "text") {
		h = slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	}
	return slog.New(contextHandler{h})
}

// ParseLevel accepts debug, info, warn or error.
func ParseLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}

func cloudLoggingKeys(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.LevelKey:
		a.Key = "severity"
		if lvl, ok := a.Value.Any().(slog.Level); ok && lvl == slog.LevelWarn {
			a.Value = slog.StringValue("WARNING")
		}
	case slog.MessageKey:
		a.Key = "message"
	case slog.TimeKey:
		a.Key = "time"
	}
	return a
}

// contextHandler adds request-scoped attributes to every record (Decorator).
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if trace, ok := ctx.Value(traceKey).(string); ok && trace != "" {
		r.AddAttrs(slog.String("logging.googleapis.com/trace", trace))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}
