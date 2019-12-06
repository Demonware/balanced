package main

import (
	"strings"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/Demonware/balanced/pkg/watcher"
)

var (
	namespace = strings.Replace(controllerName, "-", "_", -1)
	labels    = []string{"ip_pool", "vip_cidr"}
	VipUsed   = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "vip_used",
		Help:      "Total VIPs used",
	}, labels)
	VipFree = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "vip_free",
		Help:      "Total VIPs free",
	}, labels)
)

type VIPMetricsExporter struct {
}

func NewVIPMetricsExporter() *VIPMetricsExporter {
	glog.Infof("Instantiate VIPMetricsExporter")
	prometheus.MustRegister(VipUsed)
	prometheus.MustRegister(VipFree)

	prometheus.MustRegister(watcher.SyncTimeMetric)
	prometheus.MustRegister(watcher.LastSyncTime)

	return &VIPMetricsExporter{}
}

func (v *VIPMetricsExporter) Export() error {
	return nil
}
