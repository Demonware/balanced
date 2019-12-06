package main

import (
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/Demonware/balanced/pkg/watcher"
)

type DummyMetricsExporter struct {
}

func NewDummyMetricsExporter() *DummyMetricsExporter {
	glog.Infof("Instantiate DummyMetricsExporter")
	prometheus.MustRegister(watcher.SyncTimeMetric)
	prometheus.MustRegister(watcher.LastSyncTime)

	return &DummyMetricsExporter{}
}

func (v *DummyMetricsExporter) Export() error {
	return nil
}
