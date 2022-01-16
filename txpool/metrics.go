package txpool

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	prometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

// Metrics represents the txpool metrics
type Metrics struct {
	// Pending transactions
	PendingTxs metrics.Gauge
}

// GetPrometheusMetrics return the txpool metrics instance
func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := []string{}

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		PendingTxs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "txpool",
			Name:      "pending_transactions",
			Help:      "Pending transactions in the pool",
		}, labels).With(labelsWithValues...),
	}
}

// NilMetrics will return the non operational txpool metrics
func NilMetrics() *Metrics {
	return &Metrics{
		PendingTxs: discard.NewGauge(),
	}
}
