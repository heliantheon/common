package eventbus

import (
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type busMetrics struct {
	publish *prometheus.CounterVec
	consume *prometheus.CounterVec
}

func newBusMetrics(registerer prometheus.Registerer) (busMetrics, error) {
	metrics := busMetrics{
		publish: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "eventbus",
			Name:      "publish_total",
			Help:      "CloudEvents publish attempts by result.",
		}, []string{"result"}),
		consume: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "eventbus",
			Name:      "consume_total",
			Help:      "CloudEvents delivery attempts by result.",
		}, []string{"result"}),
	}
	if registerer == nil {
		return metrics, nil
	}
	if err := registerCounter(registerer, &metrics.publish); err != nil {
		return busMetrics{}, err
	}
	if err := registerCounter(registerer, &metrics.consume); err != nil {
		return busMetrics{}, err
	}
	return metrics, nil
}

func registerCounter(registerer prometheus.Registerer, counter **prometheus.CounterVec) error {
	if err := registerer.Register(*counter); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegistered) {
			return fmt.Errorf("eventbus: register metrics: %w", err)
		}
		existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.CounterVec)
		if !ok {
			return fmt.Errorf("eventbus: metric name already registered with incompatible type")
		}
		*counter = existing
	}
	return nil
}
