// Package eventbus publishes and consumes strict CloudEvents 1.0 messages on
// NATS JetStream.
package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/propagation"
)

// Config configures one service connection to NATS. It deliberately contains
// no OpenTelemetry endpoint: transport telemetry is owned by the platform.
type Config struct {
	URLs            []string
	Name            string
	Source          string
	CredentialsFile string
	Token           string
	ConnectTimeout  time.Duration
	ReconnectWait   time.Duration
	MaxReconnects   int
	Logger          *slog.Logger
	Registerer      prometheus.Registerer
}

// Ack confirms durable storage by JetStream.
type Ack struct {
	Stream    string
	Sequence  uint64
	Duplicate bool
}

// Bus owns the NATS connection, JetStream client, and SDK telemetry.
type Bus struct {
	connection *nats.Conn
	jetstream  jetstream.JetStream
	source     string
	logger     *slog.Logger
	metrics    busMetrics
}

// New connects to NATS and verifies JetStream availability.
func New(ctx context.Context, cfg Config) (*Bus, error) {
	if len(cfg.URLs) == 0 {
		return nil, fmt.Errorf("eventbus: at least one NATS URL is required")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("eventbus: connection name is required")
	}
	if strings.TrimSpace(cfg.Source) == "" {
		return nil, fmt.Errorf("eventbus: CloudEvent source is required")
	}

	connectTimeout := cfg.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}
	reconnectWait := cfg.ReconnectWait
	if reconnectWait <= 0 {
		reconnectWait = 2 * time.Second
	}
	maxReconnects := cfg.MaxReconnects
	if maxReconnects == 0 {
		maxReconnects = -1
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	options := []nats.Option{
		nats.Name(cfg.Name),
		nats.Timeout(connectTimeout),
		nats.ReconnectWait(reconnectWait),
		nats.MaxReconnects(maxReconnects),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Warn("NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			logger.Info("NATS reconnected", "server", conn.ConnectedUrl())
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			logger.Info("NATS connection closed")
		}),
	}
	if cfg.CredentialsFile != "" {
		options = append(options, nats.UserCredentials(cfg.CredentialsFile))
	}
	if cfg.Token != "" {
		options = append(options, nats.Token(cfg.Token))
	}

	connection, err := nats.Connect(strings.Join(cfg.URLs, ","), options...)
	if err != nil {
		return nil, fmt.Errorf("eventbus: connect to NATS: %w", err)
	}

	js, err := jetstream.New(connection)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("eventbus: create JetStream client: %w", err)
	}
	if _, err := js.AccountInfo(ctx); err != nil {
		connection.Close()
		return nil, fmt.Errorf("eventbus: verify JetStream: %w", err)
	}

	metrics, err := newBusMetrics(cfg.Registerer)
	if err != nil {
		connection.Close()
		return nil, err
	}

	return &Bus{
		connection: connection,
		jetstream:  js,
		source:     cfg.Source,
		logger:     logger,
		metrics:    metrics,
	}, nil
}

// Publish stores one structured CloudEvent and waits for JetStream PubAck.
func (b *Bus) Publish(ctx context.Context, subject string, input Event) (Ack, error) {
	if strings.TrimSpace(subject) == "" {
		return Ack{}, fmt.Errorf("eventbus: NATS subject is required")
	}

	event, err := buildCloudEvent(ctx, b.source, input, time.Now())
	if err != nil {
		b.metrics.publish.WithLabelValues("invalid").Inc()
		return Ack{}, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		b.metrics.publish.WithLabelValues("encode_error").Inc()
		return Ack{}, fmt.Errorf("eventbus: encode structured CloudEvent: %w", err)
	}

	message := nats.NewMsg(subject)
	message.Header.Set("Content-Type", cloudEventsContentType)
	message.Header.Set(nats.MsgIdHdr, event.ID())
	carrier := propagation.HeaderCarrier(message.Header)
	propagation.TraceContext{}.Inject(ctx, carrier)
	message.Data = payload

	ack, err := b.jetstream.PublishMsg(ctx, message)
	if err != nil {
		b.metrics.publish.WithLabelValues("error").Inc()
		return Ack{}, fmt.Errorf("eventbus: publish %s: %w", event.Type(), err)
	}
	b.metrics.publish.WithLabelValues("success").Inc()
	return Ack{Stream: ack.Stream, Sequence: ack.Sequence, Duplicate: ack.Duplicate}, nil
}

// Close gracefully drains subscriptions and buffered writes.
func (b *Bus) Close(ctx context.Context) error {
	finished := make(chan error, 1)
	go func() { finished <- b.connection.Drain() }()
	select {
	case err := <-finished:
		if err != nil {
			return fmt.Errorf("eventbus: drain NATS connection: %w", err)
		}
		return nil
	case <-ctx.Done():
		b.connection.Close()
		return fmt.Errorf("eventbus: drain NATS connection: %w", ctx.Err())
	}
}
