package network

import (
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/discard"
	prometheus "github.com/go-kit/kit/metrics/prometheus"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
)

// Metrics represents the network metrics
type Metrics struct {
	// Number of connected peers
	TotalPeerCount metrics.Gauge

	// Number of outbound connections
	OutboundConnectionsCount metrics.Gauge

	// Number of inbound connections
	InboundConnectionsCount metrics.Gauge

	// Number of pending outbound connections
	PendingOutboundConnectionsCount metrics.Gauge

	// Number of pending inbound connections
	PendingInboundConnectionsCount metrics.Gauge
}

// GetPrometheusMetrics return the network metrics instance
func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := []string{}

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		TotalPeerCount: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "network",
			Name:      "peers",
			Help:      "Number of connected peers",
		}, labels).With(labelsWithValues...),

		OutboundConnectionsCount: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "network",
			Name:      "outbound_connections_count",
			Help:      "Number of outbound connections",
		}, labels).With(labelsWithValues...),

		InboundConnectionsCount: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "network",
			Name:      "inbound_connections_count",
			Help:      "Number of inbound connections",
		}, labels).With(labelsWithValues...),

		PendingOutboundConnectionsCount: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "network",
			Name:      "pending_outbound_connections_count",
			Help:      "Number of pending outbound connections",
		}, labels).With(labelsWithValues...),

		PendingInboundConnectionsCount: prometheus.NewGaugeFrom(stdprometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "network",
			Name:      "pending_inbound_connections_count",
			Help:      "Number of pending inbound connections",
		}, labels).With(labelsWithValues...),
	}
}

// NilMetrics will return the non-operational metrics
func NilMetrics() *Metrics {
	return &Metrics{
		TotalPeerCount:                  discard.NewGauge(),
		OutboundConnectionsCount:        discard.NewGauge(),
		InboundConnectionsCount:         discard.NewGauge(),
		PendingOutboundConnectionsCount: discard.NewGauge(),
		PendingInboundConnectionsCount:  discard.NewGauge(),
	}
}
