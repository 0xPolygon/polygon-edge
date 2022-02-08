package consensus

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	prometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

// Metrics represents the consensus metrics
type Metrics struct {
	// No.of validators
	Validators metrics.Gauge
	// No.of rounds
	Rounds metrics.Gauge
	// No.of transactions in the block
	NumTxs metrics.Gauge

	//Time between current block and the previous block in seconds
	BlockInterval metrics.Gauge
}

// GetPrometheusMetrics return the consensus metrics instance
func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := []string{}

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		Validators: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "validators",
			Help:      "Number of validators.",
		}, labels).With(labelsWithValues...),
		Rounds: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "rounds",
			Help:      "Number of rounds.",
		}, labels).With(labelsWithValues...),
		NumTxs: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "num_txs",
			Help:      "Number of transactions.",
		}, labels).With(labelsWithValues...),

		BlockInterval: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "block_interval",
			Help:      "Time between current block and the previous block in seconds.",
		}, labels).With(labelsWithValues...),
	}
}

// NilMetrics will return the non operational metrics
func NilMetrics() *Metrics {
	return &Metrics{
		Validators:    discard.NewGauge(),
		Rounds:        discard.NewGauge(),
		NumTxs:        discard.NewGauge(),
		BlockInterval: discard.NewGauge(),
	}
}
