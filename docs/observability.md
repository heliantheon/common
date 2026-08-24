# Common observability contracts

## Logging

Create one `log.New` logger in the composition root and inject it. Use the
context-aware `slog` methods so an existing span context is emitted as
`trace_id` and `span_id`. The package always writes JSON to stdout and never
pushes logs over OTLP.

## Metrics

Each process creates its own `metric.Registry` and exposes `Handler()` at
`/metrics`. HTTP/gRPC RED metrics come from platform eBPF instrumentation;
services use this package for bounded business counters and histograms. Raw
paths, request IDs, trace IDs, user IDs, email addresses, and URLs are rejected
as labels.

## Event bus

Services connect directly to NATS through `eventbus.New`. `Publish` waits for a
JetStream PubAck and encodes every SDK event as CloudEvents 1.0 structured JSON
with `Content-Type: application/cloudevents+json`. Consumers ACK successful
work, delay retries, and publish metadata-only dead letters after permanent or
exhausted failures.

Business event schemas stay in their owning service. In particular, the Chaos
mail delivery event lives under `chaos/internal/mail`, not in this module.
