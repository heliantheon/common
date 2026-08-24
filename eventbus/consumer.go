package eventbus

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const deadLetterEventType = "com.heliantheon.eventbus.dead-letter.v1"

// Handler processes one decoded business event. Returning an error requests a
// delayed retry until MaxDeliver is reached.
type Handler func(context.Context, Message) error

type permanentError struct {
	err error
}

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks a handler error as non-retryable. The SDK sends sanitized
// metadata to the configured DLQ and acknowledges the original message.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// ConsumerConfig defines a durable pull consumer and its retry policy.
type ConsumerConfig struct {
	Stream        string
	Durable       string
	FilterSubject string
	DLQSubject    string
	AckWait       time.Duration
	RetryDelay    time.Duration
	MaxDeliver    int
	MaxAckPending int
}

// Subscription controls a running consumer.
type Subscription struct {
	consumer jetstream.ConsumeContext
}

// Consume creates or updates a durable consumer and begins processing.
func (b *Bus) Consume(ctx context.Context, cfg ConsumerConfig, handler Handler) (*Subscription, error) {
	if handler == nil {
		return nil, fmt.Errorf("eventbus: handler is required")
	}
	if strings.TrimSpace(cfg.Stream) == "" || strings.TrimSpace(cfg.Durable) == "" || strings.TrimSpace(cfg.FilterSubject) == "" {
		return nil, fmt.Errorf("eventbus: stream, durable, and filter subject are required")
	}
	if strings.TrimSpace(cfg.DLQSubject) == "" {
		return nil, fmt.Errorf("eventbus: DLQ subject is required")
	}
	if cfg.AckWait <= 0 {
		cfg.AckWait = 30 * time.Second
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 30 * time.Second
	}
	if cfg.MaxDeliver <= 0 {
		cfg.MaxDeliver = 5
	}
	if cfg.MaxAckPending <= 0 {
		cfg.MaxAckPending = 32
	}

	consumer, err := b.jetstream.CreateOrUpdateConsumer(ctx, cfg.Stream, jetstream.ConsumerConfig{
		Name:            cfg.Durable,
		Durable:         cfg.Durable,
		Description:     "managed by common/eventbus",
		AckPolicy:       jetstream.AckExplicitPolicy,
		AckWait:         cfg.AckWait,
		MaxDeliver:      cfg.MaxDeliver,
		FilterSubject:   cfg.FilterSubject,
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   cfg.MaxAckPending,
		Replicas:        1,
		SampleFrequency: "100%",
	})
	if err != nil {
		return nil, fmt.Errorf("eventbus: configure consumer %s: %w", cfg.Durable, err)
	}

	consumeContext, err := consumer.Consume(func(raw jetstream.Msg) {
		b.handleMessage(ctx, cfg, handler, raw)
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		b.logger.ErrorContext(ctx, "JetStream consumer error", "consumer", cfg.Durable, "error", err)
	}))
	if err != nil {
		return nil, fmt.Errorf("eventbus: start consumer %s: %w", cfg.Durable, err)
	}

	return &Subscription{consumer: consumeContext}, nil
}

// Drain stops fetching and waits for buffered handlers to finish.
func (s *Subscription) Drain(ctx context.Context) error {
	s.consumer.Drain()
	select {
	case <-s.consumer.Closed():
		return nil
	case <-ctx.Done():
		s.consumer.Stop()
		return fmt.Errorf("eventbus: drain consumer: %w", ctx.Err())
	}
}

func (b *Bus) handleMessage(ctx context.Context, cfg ConsumerConfig, handler Handler, raw jetstream.Msg) {
	metadata, err := raw.Metadata()
	if err != nil {
		b.metrics.consume.WithLabelValues("metadata_error").Inc()
		if nakErr := raw.NakWithDelay(cfg.RetryDelay); nakErr != nil {
			b.logger.ErrorContext(ctx, "JetStream metadata NAK failed", "error", nakErr)
		}
		return
	}

	messageCtx, message, err := decodeCloudEvent(ctx, raw.Data(), raw.Subject(), metadata.NumDelivered)
	if err != nil {
		b.metrics.consume.WithLabelValues("invalid").Inc()
		b.deadLetter(messageCtx, cfg.DLQSubject, deadLetter{
			NATSSubject: raw.Subject(),
			Deliveries:  metadata.NumDelivered,
			Reason:      "invalid_cloudevent",
			ErrorClass:  errorClass(err),
		}, raw)
		return
	}

	handlerErr := callHandler(messageCtx, handler, message)
	if handlerErr == nil {
		if err := raw.DoubleAck(messageCtx); err != nil {
			b.metrics.consume.WithLabelValues("ack_error").Inc()
			b.logger.ErrorContext(messageCtx, "JetStream ACK failed", "event_id", message.ID, "error", err)
			return
		}
		b.metrics.consume.WithLabelValues("success").Inc()
		return
	}
	b.logger.WarnContext(messageCtx, "CloudEvent handler failed",
		"event_id", message.ID,
		"event_type", message.Type,
		"deliveries", message.Deliveries,
		"error_class", errorClass(handlerErr),
	)

	var permanent permanentError
	if !errors.As(handlerErr, &permanent) && metadata.NumDelivered < uint64(cfg.MaxDeliver) {
		b.metrics.consume.WithLabelValues("retry").Inc()
		if err := raw.NakWithDelay(cfg.RetryDelay); err != nil {
			b.logger.ErrorContext(messageCtx, "JetStream NAK failed", "event_id", message.ID, "error", err)
		}
		return
	}

	b.metrics.consume.WithLabelValues("dead_letter").Inc()
	b.deadLetter(messageCtx, cfg.DLQSubject, deadLetter{
		EventID:     message.ID,
		EventType:   message.Type,
		EventSource: message.Source,
		NATSSubject: message.NATSSubject,
		Deliveries:  message.Deliveries,
		Reason:      "handler_failed",
		ErrorClass:  errorClass(handlerErr),
	}, raw)
}

type deadLetter struct {
	EventID     string `json:"event_id,omitempty"`
	EventType   string `json:"event_type,omitempty"`
	EventSource string `json:"event_source,omitempty"`
	NATSSubject string `json:"nats_subject"`
	Deliveries  uint64 `json:"deliveries"`
	Reason      string `json:"reason"`
	ErrorClass  string `json:"error_class,omitempty"`
}

func (b *Bus) deadLetter(ctx context.Context, subject string, data deadLetter, raw jetstream.Msg) {
	_, err := b.Publish(ctx, subject, Event{
		Type:    deadLetterEventType,
		Subject: data.EventID,
		Data:    data,
	})
	if err != nil {
		b.logger.ErrorContext(ctx, "publish dead letter failed", "event_id", data.EventID, "error", err)
		if nakErr := raw.NakWithDelay(time.Minute); nakErr != nil {
			b.logger.ErrorContext(ctx, "JetStream dead-letter NAK failed", "event_id", data.EventID, "error", nakErr)
		}
		return
	}
	if err := raw.DoubleAck(ctx); err != nil {
		b.logger.ErrorContext(ctx, "dead-letter ACK failed", "event_id", data.EventID, "error", err)
	}
}

func callHandler(ctx context.Context, handler Handler, message Message) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("eventbus handler panic (%T): %v\n%s", recovered, recovered, debug.Stack())
		}
	}()
	return handler(ctx, message)
}

func errorClass(err error) string {
	if err == nil {
		return ""
	}
	typeOf := reflect.TypeOf(err)
	if typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	return typeOf.PkgPath() + "." + typeOf.Name()
}
