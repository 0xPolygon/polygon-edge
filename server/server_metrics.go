package server

import (
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/txpool"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
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

// enableDataDogProfiler enables DataDog profiler. Enable it by setting DD_ENABLE env var.
// Additional parameters can be set with env vars (DD_) - https://docs.datadoghq.com/profiler/enabling/go/
func (s *Server) enableDataDogProfiler() error {
	if os.Getenv("DD_ENABLE") == "" {
		s.logger.Debug("DataDog profiler disabled, set DD_ENABLE env var to enable it.")

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

	// start the tracer
	tracer.Start()
	s.logger.Info("DataDog profiler started")

	return nil
}

func (s *Server) closeDataDogProfiler() {
	defer func() {
		s.logger.Debug("closing DataDog profiler")
		profiler.Stop()
	}()

	defer func() {
		s.logger.Debug("closing DataDog tracer")
		tracer.Stop()
	}()
}
