package server

import (
	"github.com/0xPolygon/polygon-sdk/consensus"
	"github.com/0xPolygon/polygon-sdk/txpool"
)

type serverMetrics struct {
	consensus *consensus.Metrics
	txpool    *txpool.Metrics
}

func metricProvider(nameSpace string, chainID string, metricsRequired bool) *serverMetrics {
	if metricsRequired {
		return &serverMetrics{
			consensus: consensus.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			txpool:    txpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}
	return &serverMetrics{
		consensus: consensus.NilMetrics(),
		txpool:    txpool.NilMetrics(),
	}

}
