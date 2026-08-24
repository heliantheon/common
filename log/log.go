// Package log provides structured application logging backed by log/slog.
//
// Log records are always JSON and are written to stdout by default so the
// platform log collector can own transport, buffering, and delivery.
package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// Config identifies the service producing log records.
type Config struct {
	Service     string
	Version     string
	Environment string
	Level       string
	AddSource   bool
	Writer      io.Writer
}

// New creates an independent structured logger. It does not install a global
// logger and does not configure an OpenTelemetry exporter.
func New(cfg Config) (*slog.Logger, error) {
	service := strings.TrimSpace(cfg.Service)
	if service == "" {
		return nil, fmt.Errorf("log: service is required")
	}

	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	writer := cfg.Writer
	if writer == nil {
		writer = os.Stdout
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		AddSource: cfg.AddSource,
		Level:     level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				attr.Key = "timestamp"
			case slog.LevelKey:
				attr.Key = "severity"
				attr.Value = slog.StringValue(strings.ToUpper(attr.Value.String()))
			case slog.MessageKey:
				attr.Key = "body"
			}
			return attr
		},
	})

	resourceAttrs := []slog.Attr{
		slog.String("service.name", service),
		slog.String("service.version", defaultValue(cfg.Version, "unknown")),
		slog.String("deployment.environment", defaultValue(cfg.Environment, "unknown")),
	}

	return slog.New(&contextHandler{next: handler.WithAttrs(resourceAttrs)}), nil
}

type contextHandler struct {
	next slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, record)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name)}
}

func parseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("log: unsupported level %q", value)
	}
}

func defaultValue(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
