package observability

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const defaultSampleRatio = 0.25

// Config contains the stable resource identity supplied by an application.
// Standard OTEL_* environment variables remain responsible for exporter and
// deployment-specific configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
}

// Init configures W3C context propagation and, when an OTLP endpoint is set,
// a batched OTLP/gRPC trace exporter. The returned function flushes pending
// spans and is always safe to call.
func Init(ctx context.Context, cfg Config) (func(), error) {
	setPropagator()

	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = strings.TrimSpace(cfg.ServiceName)
	}
	if serviceName == "" {
		return func() {}, errors.New("observability service name is required")
	}

	if !exportEnabled() {
		return func() {}, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return func() {}, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	attrs := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
		attribute.String("service.namespace", "heliantheon"),
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, attribute.String("service.version", cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, attribute.String("deployment.environment.name", cfg.Environment))
	}

	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return func() {}, fmt.Errorf("create OpenTelemetry resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(samplerFromEnvironment()),
	)
	otel.SetTracerProvider(provider)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(shutdownCtx)
	}, nil
}

func setPropagator() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

func exportEnabled() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_TRACES_EXPORTER")), "none") {
		return false
	}
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != ""
}

func samplerFromEnvironment() sdktrace.Sampler {
	ratio := defaultSampleRatio
	if raw := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG")); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed >= 0 && parsed <= 1 {
			ratio = parsed
		}
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))) {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(ratio)
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	default:
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	}
}
