package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/libnetwork/ipvs"
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	utilipvs "github.com/Demonware/balanced/pkg/util/ipvs"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/Demonware/balanced/pkg/util"
	"github.com/Demonware/balanced/pkg/watcher"
)

const (
	ServiceKind  = "service"
	EndpointKind = "endpoint"
	CreateAction = "create"
	UpdateAction = "update"
	DeleteAction = "delete"
)

var (
	namespace        = strings.Replace(controllerName, "-", "_", -1)
	serviceLabels    = []string{"hostname", "namespace", "service_name", "service_vip", "protocol", "port"}
	destLabels       = append(serviceLabels, []string{"endpoint_name", "endpoint_type", "endpoint_ip", "endpoint_port"}...)
	controllerLabels = []string{"hostname"}
	actionLabels     = append(controllerLabels, "kind", "action")
	serviceTotalConn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_total_connections",
		Help:      "Total incoming connections made",
	}, serviceLabels)
	servicePacketsIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_packets_in",
		Help:      "Total incoming packets",
	}, serviceLabels)
	servicePacketsOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_packets_out",
		Help:      "Total outgoing packets",
	}, serviceLabels)
	serviceBytesIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_bytes_in",
		Help:      "Total incoming bytes",
	}, serviceLabels)
	serviceBytesOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_bytes_out",
		Help:      "Total outgoing bytes",
	}, serviceLabels)
	servicePpsIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_pps_in",
		Help:      "Incoming packets per second",
	}, serviceLabels)
	servicePpsOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_pps_out",
		Help:      "Outgoing packets per second",
	}, serviceLabels)
	serviceCPS = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_cps",
		Help:      "Service connections per second",
	}, serviceLabels)
	serviceBpsIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_bps_in",
		Help:      "Incoming bytes per second",
	}, serviceLabels)
	serviceBpsOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_bps_out",
		Help:      "Outgoing bytes per second",
	}, serviceLabels)

	destActiveConn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_active_connections",
		Help:      "Total active incoming connections",
	}, destLabels)
	destInactiveConn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_inactive_connections",
		Help:      "Total inactive incoming connections",
	}, destLabels)
	destPersistConn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_persist_connections",
		Help:      "Total persistent incoming connections",
	}, destLabels)
	destTotalConn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_total_connections",
		Help:      "Total incoming connections made",
	}, destLabels)
	destPacketsIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_packets_in",
		Help:      "Total incoming packets",
	}, destLabels)
	destPacketsOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_packets_out",
		Help:      "Total outgoing packets",
	}, destLabels)
	destBytesIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_bytes_in",
		Help:      "Total incoming bytes",
	}, destLabels)
	destBytesOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_bytes_out",
		Help:      "Total outgoing bytes",
	}, destLabels)
	destPpsIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_pps_in",
		Help:      "Incoming packets per second",
	}, destLabels)
	destPpsOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_pps_out",
		Help:      "Outgoing packets per second",
	}, destLabels)
	destCPS = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_cps",
		Help:      "dest connections per second",
	}, destLabels)
	destBpsIn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_bps_in",
		Help:      "Incoming bytes per second",
	}, destLabels)
	destBpsOut = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "dest_bps_out",
		Help:      "Outgoing bytes per second",
	}, destLabels)

	controllerIpvsServices = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "ipvs_services",
		Help:      "Number of ipvs services in the instance",
	}, controllerLabels)
	controllerIpvsMetricsExportTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "ipvs_metrics_export_time",
		Help:      "Time it took to export metrics",
	}, controllerLabels)
	controllerIpvsProgrammedCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ipvs_programmed_total",
		Help:      "Number of IPVS rules programmed through netlink socket",
	}, actionLabels)
	controllerIpvsDesiredChangeCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "ipvs_desired_change_total",
		Help:      "Number of desired changes to IPVS rules",
	}, actionLabels)
)

type IpvsMetricsExporter struct {
	serviceManager *v1alpha1.MapServiceManager

	existingServiceMetricMap map[string][]string
	existingDestMetricMap    map[string][]string
}

// NewIpvsMetricsExporter : instantiates MetricsExporter
func NewIpvsMetricsExporter(serviceManager *v1alpha1.MapServiceManager) *IpvsMetricsExporter {
	glog.Infof("Instantiate IpvsMetricsExporter")
	prometheus.MustRegister(controllerIpvsMetricsExportTime)
	prometheus.MustRegister(controllerIpvsServices)

	prometheus.MustRegister(controllerIpvsProgrammedCount)
	prometheus.MustRegister(controllerIpvsDesiredChangeCount)

	prometheus.MustRegister(serviceBpsIn)
	prometheus.MustRegister(serviceBpsOut)
	prometheus.MustRegister(serviceBytesIn)
	prometheus.MustRegister(serviceBytesOut)
	prometheus.MustRegister(serviceCPS)
	prometheus.MustRegister(servicePacketsIn)
	prometheus.MustRegister(servicePacketsOut)
	prometheus.MustRegister(servicePpsIn)
	prometheus.MustRegister(servicePpsOut)
	prometheus.MustRegister(serviceTotalConn)

	prometheus.MustRegister(destBpsIn)
	prometheus.MustRegister(destBpsOut)
	prometheus.MustRegister(destBytesIn)
	prometheus.MustRegister(destBytesOut)
	prometheus.MustRegister(destCPS)
	prometheus.MustRegister(destPacketsIn)
	prometheus.MustRegister(destPacketsOut)
	prometheus.MustRegister(destPpsIn)
	prometheus.MustRegister(destPpsOut)
	prometheus.MustRegister(destTotalConn)
	prometheus.MustRegister(destActiveConn)
	prometheus.MustRegister(destInactiveConn)
	prometheus.MustRegister(destPersistConn)

	prometheus.MustRegister(watcher.SyncTimeMetric)
	prometheus.MustRegister(watcher.LastSyncTime)

	return &IpvsMetricsExporter{
		serviceManager:           serviceManager,
		existingServiceMetricMap: make(map[string][]string),
		existingDestMetricMap:    make(map[string][]string),
	}
}

func ipvsServiceID(s *v1alpha1.Service) string {
	address := *s.Address()
	return fmt.Sprintf("%s-%d-%d",
		address.String(), s.ProtoNumber(), s.Port())
}

func endpointID(d *ipvs.Destination) string {
	return fmt.Sprintf("%s/%d",
		d.Address.String(),
		d.Port,
	)
}

// Export : exports metrics every cycle
func (i *IpvsMetricsExporter) Export() error {
	ipvsHandle, err := ipvs.New("")
	if err != nil {
		return err
	}
	defer ipvsHandle.Close()

	glog.V(4).Infof("IpvsMetricsExporter.Export() called")
	start := time.Now()
	defer func() {
		endTime := time.Since(start)
		glog.V(4).Infof("Exporting IPVS metrics took %v", endTime)
		exportGaugeMetric(controllerIpvsMetricsExportTime, float64(endTime), util.Hostname)
	}()
	svcIDMap := map[string]*v1alpha1.Service{}
	for _, svc := range i.serviceManager.ListServicesWithAddress() {
		svcIDMap[ipvsServiceID(svc)] = svc
	}
	iSvcs, err := ipvsHandle.GetServices()
	if err != nil {
		return err
	}
	newServiceMetricMap := make(map[string][]string)
	newDestMetricMap := make(map[string][]string)

	for _, iSvc := range iSvcs {
		svc, exists := svcIDMap[utilipvs.NewIpvsService(iSvc).ID()]
		if !exists {
			continue
		}
		labelValues := []string{
			util.Hostname,
			svc.KubeAPIV1Service().Namespace,
			svc.KubeAPIV1Service().Name,
			iSvc.Address.String(),
			string(svc.Protocol()),
			strconv.Itoa(int(svc.Port())),
		}
		newServiceMetricMap[utilipvs.NewIpvsService(iSvc).ID()] = labelValues

		exportGaugeMetric(serviceBpsIn, float64(iSvc.Stats.BPSIn), labelValues...)
		exportGaugeMetric(serviceBpsOut, float64(iSvc.Stats.BPSOut), labelValues...)
		exportGaugeMetric(serviceBytesIn, float64(iSvc.Stats.BytesIn), labelValues...)
		exportGaugeMetric(serviceBytesOut, float64(iSvc.Stats.BytesOut), labelValues...)
		exportGaugeMetric(serviceCPS, float64(iSvc.Stats.CPS), labelValues...)
		exportGaugeMetric(servicePacketsIn, float64(iSvc.Stats.PacketsIn), labelValues...)
		exportGaugeMetric(servicePacketsOut, float64(iSvc.Stats.PacketsOut), labelValues...)
		exportGaugeMetric(servicePpsIn, float64(iSvc.Stats.PPSIn), labelValues...)
		exportGaugeMetric(servicePpsOut, float64(iSvc.Stats.PPSOut), labelValues...)
		exportGaugeMetric(serviceTotalConn, float64(iSvc.Stats.Connections), labelValues...)

		ipvsDests, err := ipvsHandle.GetDestinations(iSvc)
		if err != nil {
			return err
		}
		for _, ipvsDest := range ipvsDests {
			endpointKey := endpointID(ipvsDest)
			endpoint, err := svc.Get(endpointKey)
			if err != nil {
				continue
			}
			targetName := endpoint.Address().String()
			if endpoint.TargetRef() != nil {
				targetName = endpoint.TargetRef().Name
			}
			targetType := "External"
			if endpoint.IsTargetPod() {
				targetType = "Pod"
			}
			destLabelValues := append(labelValues, []string{
				targetName,
				targetType,
				endpoint.Address().String(),

				strconv.Itoa(int(endpoint.Port())),
			}...)

			newDestMetricMap[endpointKey] = destLabelValues

			exportGaugeMetric(destBpsIn, float64(ipvsDest.Stats.BPSIn), destLabelValues...)
			exportGaugeMetric(destBpsOut, float64(ipvsDest.Stats.BPSOut), destLabelValues...)
			exportGaugeMetric(destBytesIn, float64(ipvsDest.Stats.BytesIn), destLabelValues...)
			exportGaugeMetric(destBytesOut, float64(ipvsDest.Stats.BytesOut), destLabelValues...)
			exportGaugeMetric(destCPS, float64(ipvsDest.Stats.CPS), destLabelValues...)
			exportGaugeMetric(destPacketsIn, float64(ipvsDest.Stats.PacketsIn), destLabelValues...)
			exportGaugeMetric(destPacketsOut, float64(ipvsDest.Stats.PacketsOut), destLabelValues...)
			exportGaugeMetric(destPpsIn, float64(ipvsDest.Stats.PPSIn), destLabelValues...)
			exportGaugeMetric(destPpsOut, float64(ipvsDest.Stats.PPSOut), destLabelValues...)
			exportGaugeMetric(destTotalConn, float64(ipvsDest.Stats.Connections), destLabelValues...)
			exportGaugeMetric(destActiveConn, float64(ipvsDest.ActiveConnections), destLabelValues...)
			exportGaugeMetric(destInactiveConn, float64(ipvsDest.InactiveConnections), destLabelValues...)
		}
	}
	exportGaugeMetric(controllerIpvsServices, float64(len(iSvcs)), util.Hostname)

	// remove old non-existent metrics
	for svcID, labelValues := range i.existingServiceMetricMap {
		if _, exists := newServiceMetricMap[svcID]; !exists {
			glog.V(4).Infof("Deleting ipvsService metric for %s", svcID)
			serviceBpsIn.DeleteLabelValues(labelValues...)
			serviceBpsOut.DeleteLabelValues(labelValues...)
			serviceBytesIn.DeleteLabelValues(labelValues...)
			serviceBytesOut.DeleteLabelValues(labelValues...)
			serviceCPS.DeleteLabelValues(labelValues...)
			servicePacketsIn.DeleteLabelValues(labelValues...)
			servicePacketsOut.DeleteLabelValues(labelValues...)
			servicePpsIn.DeleteLabelValues(labelValues...)
			servicePpsOut.DeleteLabelValues(labelValues...)
			serviceTotalConn.DeleteLabelValues(labelValues...)
		}
	}
	i.existingServiceMetricMap = newServiceMetricMap
	for destID, labelValues := range i.existingDestMetricMap {
		if _, exists := newDestMetricMap[destID]; !exists {
			glog.V(4).Infof("Deleting ipvsDest metric for %s", destID)
			destBpsIn.DeleteLabelValues(labelValues...)
			destBpsOut.DeleteLabelValues(labelValues...)
			destBytesIn.DeleteLabelValues(labelValues...)
			destBytesOut.DeleteLabelValues(labelValues...)
			destCPS.DeleteLabelValues(labelValues...)
			destPacketsIn.DeleteLabelValues(labelValues...)
			destPacketsOut.DeleteLabelValues(labelValues...)
			destPpsIn.DeleteLabelValues(labelValues...)
			destPpsOut.DeleteLabelValues(labelValues...)
			destTotalConn.DeleteLabelValues(labelValues...)
			destActiveConn.DeleteLabelValues(labelValues...)
			destInactiveConn.DeleteLabelValues(labelValues...)
			destPersistConn.DeleteLabelValues(labelValues...)
		}
	}
	i.existingDestMetricMap = newDestMetricMap

	return nil
}

func exportGaugeMetric(gauge *prometheus.GaugeVec, value float64, lvs ...string) {
	gauge.WithLabelValues(lvs...).Set(value)
}
