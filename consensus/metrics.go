package consensus

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	prometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Validators metrics.Gauge

	Rounds metrics.Gauge

	NumTxs metrics.Gauge

	//Time between current block and the previous block in seconds
	BlockInterval metrics.Histogram
}

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

		BlockInterval: prometheus.NewHistogramFrom(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "consensus",
			Name:      "block_interval",
			Help:      "Time between current block and the previous block in seconds.",
		}, labels).With(labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		Validators:    discard.NewGauge(),
		Rounds:        discard.NewGauge(),
		NumTxs:        discard.NewGauge(),
		BlockInterval: discard.NewHistogram(),
	}
}
