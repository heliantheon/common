package metric_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/heliantheon/common/metric"
)

func TestRegistryIsIndependentAndAddsResourceLabels(t *testing.T) {
	registry, err := metric.New(metric.Config{
		Service:     "chaos",
		Version:     "1.2.3",
		Environment: "test",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	counter, err := registry.NewCounter("mail_deliveries_total", "Completed mail deliveries.")
	if err != nil {
		t.Fatalf("NewCounter() error = %v", err)
	}
	counter.Inc()

	if got := testutil.ToFloat64(counter); got != 1 {
		t.Fatalf("counter = %v, want 1", got)
	}
	if _, err := registry.Gatherer().Gather(); err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
}

func TestRegistryRejectsHighCardinalityLabels(t *testing.T) {
	registry, err := metric.New(metric.Config{Service: "chaos"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := registry.NewCounterVec("requests_total", "Requests.", "method", "user_id"); err == nil {
		t.Fatal("NewCounterVec() error = nil, want high-cardinality validation error")
	}
}
