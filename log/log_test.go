package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/trace"

	commonlog "github.com/heliantheon/common/log"
)

func TestNewWritesResourceAndTraceFields(t *testing.T) {
	var output bytes.Buffer
	logger, err := commonlog.New(commonlog.Config{
		Service:     "chaos",
		Version:     "1.2.3",
		Environment: "test",
		Writer:      &output,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
		Remote:  true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), spanContext)
	logger.InfoContext(ctx, "delivery queued", "delivery_id", "mail-1")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log record: %v", err)
	}

	want := map[string]string{
		"severity":               "INFO",
		"body":                   "delivery queued",
		"service.name":           "chaos",
		"service.version":        "1.2.3",
		"deployment.environment": "test",
		"trace_id":               spanContext.TraceID().String(),
		"span_id":                spanContext.SpanID().String(),
		"delivery_id":            "mail-1",
	}
	for key, value := range want {
		if got := record[key]; got != value {
			t.Errorf("record[%q] = %v, want %q", key, got, value)
		}
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	if _, err := commonlog.New(commonlog.Config{}); err == nil {
		t.Fatal("New() error = nil, want service validation error")
	}
	if _, err := commonlog.New(commonlog.Config{Service: "chaos", Level: "verbose"}); err == nil {
		t.Fatal("New() error = nil, want level validation error")
	}
}
