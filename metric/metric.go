// Package metric provides an independent Prometheus registry for a service.
package metric

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var forbiddenLabels = map[string]struct{}{
	"email": {}, "ip": {}, "path": {}, "request_id": {}, "span_id": {},
	"trace_id": {}, "uri": {}, "url": {}, "user": {}, "user_id": {},
}

// Config identifies metrics emitted by the service.
type Config struct {
	Service     string
	Version     string
	Environment string
	Namespace   string
}

// Registry owns one service's collectors and exposition handler.
type Registry struct {
	registry    *prometheus.Registry
	namespace   string
	constLabels prometheus.Labels
}

// New constructs a registry without mutating Prometheus' global registry.
func New(cfg Config) (*Registry, error) {
	service := strings.TrimSpace(cfg.Service)
	if service == "" {
		return nil, fmt.Errorf("metric: service is required")
	}

	namespace := strings.TrimSpace(cfg.Namespace)
	if namespace == "" {
		namespace = service
	}

	registry := prometheus.NewRegistry()
	if err := registry.Register(collectors.NewGoCollector()); err != nil {
		return nil, fmt.Errorf("metric: register Go collector: %w", err)
	}
	if err := registry.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return nil, fmt.Errorf("metric: register process collector: %w", err)
	}

	return &Registry{
		registry:  registry,
		namespace: namespace,
		constLabels: prometheus.Labels{
			"service":     service,
			"version":     defaultValue(cfg.Version, "unknown"),
			"environment": defaultValue(cfg.Environment, "unknown"),
		},
	}, nil
}

// Handler exposes this registry in the Prometheus text format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

// Gatherer returns the native Prometheus gatherer for integrations that need it.
func (r *Registry) Gatherer() prometheus.Gatherer { return r.registry }

// Registerer returns the native Prometheus registerer for advanced collectors.
func (r *Registry) Registerer() prometheus.Registerer { return r.registry }

// NewCounter registers a counter with common resource labels.
func (r *Registry) NewCounter(name, help string) (prometheus.Counter, error) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   r.namespace,
		Name:        name,
		Help:        help,
		ConstLabels: r.constLabels,
	})
	if err := r.registry.Register(counter); err != nil {
		return nil, fmt.Errorf("metric: register counter %q: %w", name, err)
	}
	return counter, nil
}

// NewCounterVec registers a labeled counter after rejecting unsafe label names.
func (r *Registry) NewCounterVec(name, help string, labels ...string) (*prometheus.CounterVec, error) {
	if err := validateLabels(labels); err != nil {
		return nil, err
	}
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   r.namespace,
		Name:        name,
		Help:        help,
		ConstLabels: r.constLabels,
	}, labels)
	if err := r.registry.Register(counter); err != nil {
		return nil, fmt.Errorf("metric: register counter vector %q: %w", name, err)
	}
	return counter, nil
}

// NewHistogram registers a histogram with common resource labels.
func (r *Registry) NewHistogram(name, help string, buckets []float64) (prometheus.Histogram, error) {
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   r.namespace,
		Name:        name,
		Help:        help,
		ConstLabels: r.constLabels,
		Buckets:     buckets,
	})
	if err := r.registry.Register(histogram); err != nil {
		return nil, fmt.Errorf("metric: register histogram %q: %w", name, err)
	}
	return histogram, nil
}

// NewHistogramVec registers a labeled histogram after rejecting unsafe label names.
func (r *Registry) NewHistogramVec(name, help string, buckets []float64, labels ...string) (*prometheus.HistogramVec, error) {
	if err := validateLabels(labels); err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   r.namespace,
		Name:        name,
		Help:        help,
		ConstLabels: r.constLabels,
		Buckets:     buckets,
	}, labels)
	if err := r.registry.Register(histogram); err != nil {
		return nil, fmt.Errorf("metric: register histogram vector %q: %w", name, err)
	}
	return histogram, nil
}

func validateLabels(labels []string) error {
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			return fmt.Errorf("metric: label name is empty")
		}
		if _, forbidden := forbiddenLabels[label]; forbidden {
			return fmt.Errorf("metric: high-cardinality label %q is forbidden", label)
		}
		if _, duplicate := seen[label]; duplicate {
			return fmt.Errorf("metric: duplicate label %q", label)
		}
		seen[label] = struct{}{}
	}
	return nil
}

func defaultValue(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
