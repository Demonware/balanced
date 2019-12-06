package metrics

import (
	_ "net/http/pprof"

	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsPublisher struct {
	BindAddress      string
	Path             string
	Port             string
	ExportInterval   time.Duration
	MetricsExporters []MetricsExporter
}

func (mp *MetricsPublisher) Register(metricsExporter MetricsExporter) {
	// TODO: add duplicate exporter detection
	mp.MetricsExporters = append(mp.MetricsExporters, metricsExporter)
}

func (mp *MetricsPublisher) Export() {
	for _, mp := range mp.MetricsExporters {
		go func(mp MetricsExporter) {
			err := mp.Export()
			if err != nil {
				glog.Errorf("MetricsExporter error: %s", err.Error())
			}
		}(mp)
	}
}

func (mp *MetricsPublisher) Run(stopc <-chan struct{}) {
	glog.Info("Starting Metrics Publisher")

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", mp.BindAddress, mp.Port),
		Handler: http.DefaultServeMux,
	}

	http.Handle(mp.Path, promhttp.Handler())

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil {
			glog.Errorf("MetricsPublisher HTTP server error: %s", err.Error())
		}
	}()

	ticker := time.NewTicker(mp.ExportInterval)
	defer ticker.Stop()

	mp.Export() // call publish first
	go func() {
		for t := range ticker.C {
			glog.V(4).Infof("ExportMetrics Tick at %s", t)
			mp.Export()
		}
	}()

	<-stopc
	glog.Info("Stopping Metrics Publisher")
}

func NewMetricsPublisher(bindAddr string, path string, port string, publishInterval time.Duration) *MetricsPublisher {
	return &MetricsPublisher{
		BindAddress:    bindAddr,
		Path:           path,
		Port:           port,
		ExportInterval: publishInterval,
	}
}
