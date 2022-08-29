// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package profiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"time"
)

type point struct {
	metric string
	value  float64
}

// MarshalJSON serialize points as array tuples
func (p point) MarshalJSON() ([]byte, error) {
	return json.Marshal([]interface{}{
		p.metric,
		p.value,
	})
}

type collectionTooFrequent struct {
	min      time.Duration
	observed time.Duration
}

func (e collectionTooFrequent) Error() string {
	return fmt.Sprintf("period between metrics collection is too small min=%v observed=%v", e.min, e.observed)
}

type metrics struct {
	collectedAt time.Time
	stats       runtime.MemStats
	compute     func(*runtime.MemStats, *runtime.MemStats, time.Duration, time.Time) []point
}

func newMetrics() *metrics {
	return &metrics{
		compute: computeMetrics,
	}
}

func (m *metrics) reset(now time.Time) {
	m.collectedAt = now
	runtime.ReadMemStats(&m.stats)
}

func (m *metrics) report(now time.Time, buf *bytes.Buffer) error {
	period := now.Sub(m.collectedAt)

	if period < time.Second {
		// Profiler could be mis-configured to report more frequently than every second
		// or a system clock issue causes time to run backwards.
		// We can't emit valid metrics in either case.
		return collectionTooFrequent{min: time.Second, observed: period}
	}

	previousStats := m.stats
	m.reset(now)

	points := m.compute(&previousStats, &m.stats, period, now)
	data, err := json.Marshal(removeInvalid(points))

	if err != nil {
		// NB the minimum period check and removeInvalid ensures we don't hit this case
		return err
	}

	if _, err := buf.Write(data); err != nil {
		return err
	}

	return nil
}

func computeMetrics(prev *runtime.MemStats, curr *runtime.MemStats, period time.Duration, now time.Time) []point {
	return []point{
		{metric: "go_alloc_bytes_per_sec", value: rate(curr.TotalAlloc, prev.TotalAlloc, period/time.Second)},
		{metric: "go_allocs_per_sec", value: rate(curr.Mallocs, prev.Mallocs, period/time.Second)},
		{metric: "go_frees_per_sec", value: rate(curr.Frees, prev.Frees, period/time.Second)},
		{metric: "go_heap_growth_bytes_per_sec", value: rate(curr.HeapAlloc, prev.HeapAlloc, period/time.Second)},
		{metric: "go_gcs_per_sec", value: rate(uint64(curr.NumGC), uint64(prev.NumGC), period/time.Second)},
		{metric: "go_gc_pause_time", value: rate(curr.PauseTotalNs, prev.PauseTotalNs, period)}, // % of time spent paused
		{metric: "go_max_gc_pause_time", value: float64(maxPauseNs(curr, now.Add(-period)))},
	}
}

func rate(curr, prev uint64, period time.Duration) float64 {
	return float64(int64(curr)-int64(prev)) / float64(period)
}

// maxPauseNs returns maximum pause time within the recent period, assumes stats populated at period end
func maxPauseNs(stats *runtime.MemStats, periodStart time.Time) (max uint64) {
	// NB
	// stats.PauseEnd is a circular buffer of recent GC pause end times as nanoseconds since the epoch.
	// stats.PauseNs is a circular buffer of recent GC pause times in nanoseconds.
	// The most recent pause is indexed by (stats.NumGC+255)%256

	for i := 0; i < 256; i++ {
		offset := (int(stats.NumGC) + 255 - i) % 256
		// Stop searching if we find a PauseEnd outside the period
		if time.Unix(0, int64(stats.PauseEnd[offset])).Before(periodStart) {
			break
		}
		if stats.PauseNs[offset] > max {
			max = stats.PauseNs[offset]
		}
	}
	return max
}

// removeInvalid removes NaN and +/-Inf values as they can't be json-serialized
// This is an extra safety check to ensure we don't emit bad data in case of
// a metric computation coding error
func removeInvalid(points []point) (result []point) {
	for _, p := range points {
		if math.IsNaN(p.value) || math.IsInf(p.value, 0) {
			continue
		}
		result = append(result, p)
	}
	return result
}
