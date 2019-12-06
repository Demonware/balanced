package watcher

import "github.com/prometheus/client_golang/prometheus"

var (
	SyncTimeMetric = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "balanced_controllers",
		Name:      "sync_time",
		Help:      "Controller Sync Time",
		Buckets:   prometheus.ExponentialBuckets(100000, float64(2), 20), // 0.1ms to ~105s
	})
	LastSyncTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "balanced_controllers",
		Name:      "sync_time_last",
		Help:      "Controller Last Sync Time",
	})
)
