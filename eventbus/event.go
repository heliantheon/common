package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/nats-io/nuid"
	"go.opentelemetry.io/otel/propagation"
)

const cloudEventsContentType = "application/cloudevents+json"

// Event contains the business-owned attributes of a CloudEvent. The SDK owns
// specversion, source, time, content type, encoding, and trace propagation.
type Event struct {
	ID         string
	Type       string
	Subject    string
	DataSchema string
	Data       any
	Extensions map[string]any
}

// Message is the decoded, transport-independent event delivered to a handler.
type Message struct {
	ID          string
	Type        string
	Source      string
	Subject     string
	Time        time.Time
	DataSchema  string
	Data        json.RawMessage
	Extensions  map[string]any
	NATSSubject string
	Deliveries  uint64
}

// Decode unmarshals the CloudEvent data into a business type.
func (m Message) Decode(target any) error {
	if target == nil {
		return fmt.Errorf("eventbus: decode target is nil")
	}
	if err := json.Unmarshal(m.Data, target); err != nil {
		return fmt.Errorf("eventbus: decode %s data: %w", m.Type, err)
	}
	return nil
}

func buildCloudEvent(ctx context.Context, source string, input Event, now time.Time) (cloudevents.Event, error) {
	if strings.TrimSpace(input.Type) == "" {
		return cloudevents.Event{}, fmt.Errorf("eventbus: event type is required")
	}

	event := cloudevents.NewEvent(cloudevents.VersionV1)
	event.SetID(defaultString(input.ID, nuid.Next()))
	event.SetSource(source)
	event.SetType(strings.TrimSpace(input.Type))
	event.SetTime(now.UTC())
	if input.Subject != "" {
		event.SetSubject(input.Subject)
	}
	if input.DataSchema != "" {
		event.SetDataSchema(input.DataSchema)
	}
	for name, value := range input.Extensions {
		event.SetExtension(strings.ToLower(name), value)
	}

	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	for _, name := range []string{"traceparent", "tracestate"} {
		if value := carrier.Get(name); value != "" {
			event.SetExtension(name, value)
		}
	}

	if err := event.SetData(cloudevents.ApplicationJSON, input.Data); err != nil {
		return cloudevents.Event{}, fmt.Errorf("eventbus: encode event data: %w", err)
	}
	if err := event.Validate(); err != nil {
		return cloudevents.Event{}, fmt.Errorf("eventbus: invalid CloudEvent: %w", err)
	}
	return event, nil
}

func decodeCloudEvent(ctx context.Context, payload []byte, natsSubject string, deliveries uint64) (context.Context, Message, error) {
	var event cloudevents.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return ctx, Message{}, fmt.Errorf("eventbus: decode CloudEvent: %w", err)
	}
	if event.SpecVersion() != cloudevents.VersionV1 {
		return ctx, Message{}, fmt.Errorf("eventbus: unsupported CloudEvent specversion %q", event.SpecVersion())
	}
	if err := event.Validate(); err != nil {
		return ctx, Message{}, fmt.Errorf("eventbus: invalid CloudEvent: %w", err)
	}
	if event.DataContentType() != cloudevents.ApplicationJSON {
		return ctx, Message{}, fmt.Errorf("eventbus: unsupported datacontenttype %q", event.DataContentType())
	}

	extensions := event.Extensions()
	carrier := propagation.MapCarrier{}
	for _, name := range []string{"traceparent", "tracestate"} {
		if value, ok := extensions[name].(string); ok {
			carrier.Set(name, value)
		}
	}
	ctx = propagation.TraceContext{}.Extract(ctx, carrier)

	return ctx, Message{
		ID:          event.ID(),
		Type:        event.Type(),
		Source:      event.Source(),
		Subject:     event.Subject(),
		Time:        event.Time(),
		DataSchema:  event.DataSchema(),
		Data:        append(json.RawMessage(nil), event.Data()...),
		Extensions:  extensions,
		NATSSubject: natsSubject,
		Deliveries:  deliveries,
	}, nil
}

func defaultString(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
