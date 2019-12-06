package main

import (
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/Demonware/balanced/pkg/watcher"
)

type TunnelMetricsExporter struct {
}

func NewTunnelMetricsExporter() *TunnelMetricsExporter {
	glog.Infof("Instantiate TunnelMetricsExporter")
	prometheus.MustRegister(watcher.SyncTimeMetric)
	prometheus.MustRegister(watcher.LastSyncTime)

	return &TunnelMetricsExporter{}
}

func (v *TunnelMetricsExporter) Export() error {
	return nil
}
