package observability

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/stats"

	"github.com/heliantheon/common/logger"
)

// TraceIDHeader exposes the server trace identifier without exposing tracing
// backend details or accepting it as an authorization signal.
const TraceIDHeader = "X-Trace-ID"

// GinMiddleware returns the shared HTTP tracing and structured access logging
// middleware in the required order. Health probes are intentionally excluded
// to keep traces and logs useful. New services should inject their slog logger;
// callers that omit it keep using the legacy Zap logger.
func GinMiddleware(serviceName string, loggers ...*slog.Logger) []gin.HandlerFunc {
	var structuredLogger *slog.Logger
	if len(loggers) > 0 {
		structuredLogger = loggers[0]
	}
	return []gin.HandlerFunc{
		otelgin.Middleware(serviceName, otelgin.WithFilter(func(req *http.Request) bool {
			return req.URL.Path != "/health"
		})),
		traceResponseHeader(),
		accessLogger(structuredLogger),
	}
}

// GRPCClientStatsHandler propagates trace context and records client spans.
func GRPCClientStatsHandler() stats.Handler {
	return otelgrpc.NewClientHandler()
}

// GRPCServerStatsHandler extracts trace context and records server spans.
func GRPCServerStatsHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

func traceResponseHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		spanContext := trace.SpanContextFromContext(c.Request.Context())
		if spanContext.IsValid() {
			c.Header(TraceIDHeader, spanContext.TraceID().String())
		}
		c.Next()
	}
}

func accessLogger(structuredLogger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		startedAt := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		if structuredLogger != nil {
			fields := []any{
				slog.String("http.request.method", c.Request.Method),
				slog.String("http.route", route),
				slog.Int("http.response.status_code", c.Writer.Status()),
				slog.Int64("http.server.request.duration_ms", time.Since(startedAt).Milliseconds()),
				slog.String("client.address", c.ClientIP()),
			}
			if len(c.Errors) > 0 {
				fields = append(fields, slog.String("error", c.Errors.String()))
				structuredLogger.ErrorContext(c.Request.Context(), "HTTP request completed", fields...)
				return
			}
			structuredLogger.InfoContext(c.Request.Context(), "HTTP request completed", fields...)
			return
		}

		legacyFields := []zap.Field{
			zap.String("http.request.method", c.Request.Method),
			zap.String("http.route", route),
			zap.Int("http.response.status_code", c.Writer.Status()),
			zap.Int64("http.server.request.duration_ms", time.Since(startedAt).Milliseconds()),
			zap.String("client.address", c.ClientIP()),
		}
		requestLogger := logger.WithContext(c.Request.Context())
		if len(c.Errors) > 0 {
			requestLogger.Error("HTTP request completed", append(legacyFields, zap.String("error", c.Errors.String()))...)
			return
		}
		requestLogger.Info("HTTP request completed", legacyFields...)
	}
}
