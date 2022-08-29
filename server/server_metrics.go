package server

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/txpool"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
	"os"
)

// serverMetrics holds the metric instances of all sub systems
type serverMetrics struct {
	consensus *consensus.Metrics
	network   *network.Metrics
	txpool    *txpool.Metrics
}

// metricProvider serverMetric instance for the given ChainID and nameSpace
func metricProvider(nameSpace string, chainID string, metricsRequired bool) *serverMetrics {
	if metricsRequired {
		return &serverMetrics{
			consensus: consensus.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			network:   network.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
			txpool:    txpool.GetPrometheusMetrics(nameSpace, "chain_id", chainID),
		}
	}

	return &serverMetrics{
		consensus: consensus.NilMetrics(),
		network:   network.NilMetrics(),
		txpool:    txpool.NilMetrics(),
	}
}

// enableDataDogMetrics enables DataDog profiler. Enable it by setting DD_ENABLE env var.
// Additional parameters can be set with env vars (DD_) - https://docs.datadoghq.com/profiler/enabling/go/
func (s *Server) enableDataDogMetrics() error {
	if os.Getenv("DD_ENABLE") == "" {
		s.logger.Debug("DD_ENABLE environment variable not found, " +
			"datadog profiler will not be initialized")

		return nil
	}

	if err := profiler.Start(
		// enable all profiles
		profiler.WithProfileTypes(
			profiler.CPUProfile,
			profiler.HeapProfile,
			profiler.BlockProfile,
			profiler.MutexProfile,
			profiler.GoroutineProfile,
			profiler.MetricsProfile,
		),
	); err != nil {
		return fmt.Errorf("could not start datadog profiler: %w", err)
	}
	defer profiler.Stop()

	return nil
}
