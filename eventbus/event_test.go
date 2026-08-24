package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestCloudEventRoundTripIsStructuredV1(t *testing.T) {
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{2},
		Remote:  true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), spanContext)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	event, err := buildCloudEvent(ctx, "urn:heliantheon:chaos", Event{
		ID:      "delivery-1",
		Type:    "com.heliantheon.chaos.mail.delivery.requested.v1",
		Subject: "delivery-1",
		Data:    map[string]any{"template_id": "otp"},
	}, now)
	if err != nil {
		t.Fatalf("buildCloudEvent() error = %v", err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	for key, want := range map[string]any{
		"specversion": "1.0",
		"id":          "delivery-1",
		"source":      "urn:heliantheon:chaos",
		"type":        "com.heliantheon.chaos.mail.delivery.requested.v1",
	} {
		if got := envelope[key]; got != want {
			t.Errorf("envelope[%q] = %v, want %v", key, got, want)
		}
	}
	if envelope["traceparent"] == "" {
		t.Fatal("traceparent extension is empty")
	}

	decodedCtx, message, err := decodeCloudEvent(context.Background(), payload, "events.chaos.mail", 2)
	if err != nil {
		t.Fatalf("decodeCloudEvent() error = %v", err)
	}
	if got := trace.SpanContextFromContext(decodedCtx); got.TraceID() != spanContext.TraceID() {
		t.Fatalf("decoded trace ID = %s, want %s", got.TraceID(), spanContext.TraceID())
	}
	if message.Deliveries != 2 || message.NATSSubject != "events.chaos.mail" {
		t.Fatalf("message transport metadata = %#v", message)
	}
	var data struct {
		TemplateID string `json:"template_id"`
	}
	if err := message.Decode(&data); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if data.TemplateID != "otp" {
		t.Fatalf("TemplateID = %q, want otp", data.TemplateID)
	}
}

func TestDeadLetterDoesNotContainPayloadOrErrorMessage(t *testing.T) {
	data := deadLetter{
		EventID:     "delivery-1",
		EventType:   "mail.requested.v1",
		EventSource: "chaos",
		NATSSubject: "events.chaos.mail",
		Deliveries:  5,
		Reason:      "handler_failed",
		ErrorClass:  errorClass(errors.New("secret OTP 123456")),
	}
	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(payload) == "" || json.Valid(payload) == false {
		t.Fatalf("invalid dead letter JSON: %s", payload)
	}
	if strings.Contains(string(payload), "secret OTP 123456") {
		t.Fatal("dead letter contains error message")
	}
}
