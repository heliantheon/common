package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestGinMiddlewareContinuesIncomingTrace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalProvider := otel.GetTracerProvider()
	originalPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(originalProvider)
		otel.SetTextMapPropagator(originalPropagator)
	})

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	router := gin.New()
	router.Use(GinMiddleware("test-service")...)
	router.GET("/api/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/test", nil)
	request.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get(TraceIDHeader); got != traceID {
		t.Fatalf("%s = %q, want %q", TraceIDHeader, got, traceID)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	if got := spans[0].SpanContext().TraceID().String(); got != traceID {
		t.Fatalf("span trace ID = %q, want %q", got, traceID)
	}
	if !spans[0].Parent().IsRemote() {
		t.Fatal("expected a remote parent extracted from traceparent")
	}
}

func TestGinMiddlewareExcludesHealthProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalProvider := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(originalProvider) })

	recorder := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))

	router := gin.New()
	router.Use(GinMiddleware("test-service")...)
	router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil))

	if len(recorder.Ended()) != 0 {
		t.Fatalf("health probe created %d spans, want 0", len(recorder.Ended()))
	}
}
