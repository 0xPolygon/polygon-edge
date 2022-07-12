package server

import (
	"github.com/0xPolygon/polygon-edge/backend"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/txpool"
)

// serverMetrics holds the metric instances of all sub systems
type serverMetrics struct {
	consensus *backend.Metrics
	network   *network.Metrics
	txpool    *txpool.Metrics
}

// metricProvider serverMetric instance for the given ChainID and nameSpace
func metricProvider(nameSpace string, chainID string, metricsRequired bool) *serverMetrics {
	if metricsRequired {
		return &serverMetrics{
			consensus: backend.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			network:   network.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			txpool:    txpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &serverMetrics{
		consensus: backend.NilMetrics(),
		network:   network.NilMetrics(),
		txpool:    txpool.NilMetrics(),
	}
}
