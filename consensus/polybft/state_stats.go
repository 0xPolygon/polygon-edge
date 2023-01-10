package polybft

import (
	"time"

	"github.com/armon/go-metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// startStatsReleasing starts the process that releases BoltDB stats into prometheus periodically.
func (s *State) startStatsReleasing() {
	const (
		statUpdatePeriod = 10 * time.Second
		dbSubsystem      = "db"
		txSubsystem      = "tx"
		namespace        = "polybft_state"
	)

	// Grab the initial stats.
	prev := s.db.Stats()

	// Initialize ticker in order to send stats once a statUpdatePeriod
	ticker := time.NewTicker(statUpdatePeriod)

	// Stop ticker (stats releasing basically) when receiving the closing signal
	go func() {
		<-s.close
		ticker.Stop()
	}()

	for range ticker.C {
		// Grab the current stats and diff them.
		stats := s.db.Stats()
		diff := stats.Sub(&prev)

		// Freelist stats
		{
			// Update total number of free pages on the freelist
			metrics.SetGauge(
				[]string{prometheus.BuildFQName(namespace, dbSubsystem, "freelist_free_pages")},
				float32(diff.FreePageN),
			)

			// Update total number of pending pages on the freelist
			metrics.SetGauge(
				[]string{prometheus.BuildFQName(namespace, dbSubsystem, "freelist_pending_pages")},
				float32(diff.PendingPageN),
			)

			// Update total bytes allocated in free pages
			metrics.SetGauge(
				[]string{prometheus.BuildFQName(namespace, dbSubsystem, "freelist_free_page_allocated_bytes")},
				float32(diff.FreeAlloc),
			)

			// Update total bytes used by the freelist
			metrics.SetGauge(
				[]string{prometheus.BuildFQName(namespace, dbSubsystem, "freelist_in_use_bytes")},
				float32(diff.FreelistInuse),
			)
		}

		// Transaction stats
		{
			// Update total number of started read transactions
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, dbSubsystem, "read_tx_total")},
				float32(diff.TxN),
			)

			// Update number of currently open read transactions
			metrics.SetGauge(
				[]string{prometheus.BuildFQName(namespace, dbSubsystem, "open_read_tx")},
				float32(diff.OpenTxN),
			)
		}

		// Global, ongoing stats
		{
			// Update number of page allocations
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "pages_allocated_total")},
				float32(diff.TxStats.PageCount),
			)

			// Update total bytes allocated
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "pages_allocated_bytes_total")},
				float32(diff.TxStats.PageAlloc),
			)

			// Update number of cursors created
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "cursors_total")},
				float32(diff.TxStats.CursorCount),
			)

			// Update number of node allocations
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "nodes_allocated_total")},
				float32(diff.TxStats.NodeCount),
			)

			// Update number of node dereferences
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "nodes_dereferenced_total")},
				float32(diff.TxStats.NodeDeref),
			)

			// Update number of node rebalances
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "node_rebalances_total")},
				float32(diff.TxStats.Rebalance),
			)

			// Update total time spent rebalancing
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "node_rebalance_seconds_total")},
				float32(diff.TxStats.RebalanceTime.Seconds()),
			)

			// Update number of nodes split
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "nodes_split_total")},
				float32(diff.TxStats.Split),
			)

			// Update number of nodes spilled
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "nodes_spilled_total")},
				float32(diff.TxStats.Spill),
			)

			// Update total time spent spilling
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "nodes_spilled_seconds_total")},
				float32(diff.TxStats.SpillTime.Seconds()),
			)

			// Update number of writes performed
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "writes_total")},
				float32(diff.TxStats.Write),
			)

			// Update total time spent writing to disk
			metrics.IncrCounter(
				[]string{prometheus.BuildFQName(namespace, txSubsystem, "write_seconds_total")},
				float32(diff.TxStats.WriteTime),
			)
		}

		// Save stats for the next loop.
		prev = stats
	}
}
