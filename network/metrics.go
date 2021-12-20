package network

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	prometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

// Metrics represents the network metrics
type Metrics struct {
	// No.of connected peers
	Peers metrics.Gauge
}

// GetPrometheusMetrics return the network metrics instance
func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := []string{}

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		Peers: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "network",
			Name:      "peers",
			Help:      "Number of connected peers.",
		}, labels).With(labelsWithValues...),
	}
}

// NilMetrics will return the non operational metrics
func NilMetrics() *Metrics {
	return &Metrics{
		Peers: discard.NewGauge(),
	}
}
