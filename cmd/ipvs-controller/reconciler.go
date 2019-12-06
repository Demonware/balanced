package main

import (
	"fmt"
	"net"
	"syscall"

	utilipvs "github.com/Demonware/balanced/pkg/util/ipvs"

	"github.com/docker/libnetwork/ipvs"
	"github.com/golang/glog"
	"github.com/sasha-s/go-deadlock"
	"github.com/Demonware/balanced/pkg/util"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	api "k8s.io/api/core/v1"
)

const (
	ipvsSchedulerAnnotation = "balanced.demonware.net/ipvs-scheduler"
)

type IpvsReconciler struct {
	cidrs                []*net.IPNet
	eventRecorder        record.EventRecorder
	serviceInformer      cache.SharedIndexInformer
	sm                   v1alpha1.ServiceManager
	defaultIpvsScheduler string

	m deadlock.Mutex
}

func NewIpvsReconciler(cidrs []*net.IPNet, s cache.SharedIndexInformer, eventRecorder record.EventRecorder,
	sm v1alpha1.ServiceManager, defaultIpvsScheduler string) *IpvsReconciler {

	return &IpvsReconciler{
		cidrs:                cidrs,
		serviceInformer:      s,
		eventRecorder:        eventRecorder,
		sm:                   sm,
		defaultIpvsScheduler: defaultIpvsScheduler,
	}
}

func (ir *IpvsReconciler) upsertIpvsService(svc *v1alpha1.Service) error {
	return withIpvsHandle(func(ipvsHandle *ipvs.Handle) error {
		newiSvc := ir.ConvertServiceToIpvsService(svc)
		if existingiSvc, err := ipvsHandle.GetService(newiSvc); err == nil {
			if ir.hasIpvsServiceChanged(existingiSvc, svc) {
				err := ipvsHandle.UpdateService(newiSvc)
				if err != nil {
					return err
				}
				controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, ServiceKind, UpdateAction).Inc()
				ir.eventRecorder.Eventf(
					svc.KubeAPIV1Service(), api.EventTypeNormal,
					"UpdatedIPVSService", "Updated IPVSService %s in IPVS Node",
					utilipvs.NewIpvsService(existingiSvc).ID())
			}
		} else {
			err := ipvsHandle.NewService(newiSvc)
			if err != nil {
				return err
			}
			controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, ServiceKind, CreateAction).Inc()
			ir.eventRecorder.Eventf(
				svc.KubeAPIV1Service(), api.EventTypeNormal,
				"CreatedIPVSService", "Added IPVSService %s to IPVS",
				utilipvs.NewIpvsService(newiSvc).ID())
		}
		return nil
	})
}

func (ir *IpvsReconciler) deleteIpvsService(svc *v1alpha1.Service) error {
	return withIpvsHandle(func(ipvsHandle *ipvs.Handle) error {
		iSvc := ir.ConvertServiceToIpvsService(svc)
		if ipvsHandle.IsServicePresent(iSvc) {
			err := ipvsHandle.DelService(iSvc)
			if err != nil {
				return err
			}
			controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, ServiceKind, DeleteAction).Inc()
			ir.eventRecorder.Eventf(
				svc.KubeAPIV1Service(), api.EventTypeNormal,
				"DeletedIPVSService", "Deleted IPVSService %s in IPVS Node",
				utilipvs.NewIpvsService(iSvc).ID())
			return nil
		}
		glog.Infof("deleteIpvsService warning: Service does not exist in IPVS Node: %s", svc.ID())
		return nil
	})
}

func (ir *IpvsReconciler) isIPinCIDRs(ip net.IP) bool {
	for _, cidr := range ir.cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func (ir *IpvsReconciler) isValidIPAddress(svc *v1alpha1.Service) bool {
	if svc == nil {
		return false
	}
	if svc.Address() == nil {
		return false
	}
	if !ir.isIPinCIDRs(*svc.Address()) {
		return false
	}
	return true
}

func (ir *IpvsReconciler) OnChangeServiceHandler(prevSvc *v1alpha1.Service, svc *v1alpha1.Service) {
	glog.V(4).Infof("IpvsReconciler.OnChangeServiceHandler called")
	if prevSvc != nil {
		if svc != nil {
			if ir.isValidIPAddress(svc) {
				controllerIpvsDesiredChangeCount.WithLabelValues(util.Hostname, ServiceKind, UpdateAction).Inc()
				ir.upsertServiceHandler(svc) // update case
			}
		} else if ir.isValidIPAddress(prevSvc) {
			controllerIpvsDesiredChangeCount.WithLabelValues(util.Hostname, ServiceKind, DeleteAction).Inc()
			ir.deleteServiceHandler(prevSvc) // delete case
		}
	} else if ir.isValidIPAddress(svc) {
		controllerIpvsDesiredChangeCount.WithLabelValues(util.Hostname, ServiceKind, CreateAction).Inc()
		ir.upsertServiceHandler(svc) // insert case
	}
}

func (ir *IpvsReconciler) upsertServiceHandler(svc *v1alpha1.Service) {
	err := ir.upsertIpvsService(svc)
	if err != nil {
		glog.Infof("IpvsReconciler.upsertServiceHandler error: %s", err)
		return
	}
	vipIface, err := util.GetDummyVipInterface()
	if err != nil {
		glog.Infof("Failed getting dummy interface: %s", err)
		return
	}
	_, err = util.AddIPtoInterface(*svc.Address(), vipIface)
	if err != nil {
		glog.Infof("Failed to add IP address to dummy interface: %s (%s)", *svc.Address(), err)
	}
}

func (ir *IpvsReconciler) deleteServiceHandler(prevSvc *v1alpha1.Service) {
	glog.Infof("IpvsReconciler.deleteServiceHandler called: %s", prevSvc.ID())
	err := ir.deleteIpvsService(prevSvc)
	if err != nil {
		glog.Infof("IpvsReconciler.upsertServiceHandler error: %s", err)
		return
	}
	vipIface, err := util.GetDummyVipInterface()
	if err != nil {
		glog.Infof("Failed getting dummy interface: %s", err)
		return
	}
	err = util.DeleteIPfromInterface(*prevSvc.Address(), vipIface)
	if err != nil {
		glog.Infof("Failed to remove IP address from dummy interface: %s (%s)", *prevSvc.Address(), err)
	}
}

func (ir *IpvsReconciler) OnChangeEndpointHandler(prevE *v1alpha1.Endpoint, e *v1alpha1.Endpoint) {
	glog.V(4).Infof("IpvsReconciler.OnChangeEndpointHandler called")
	if prevE != nil {
		if e != nil {
			// update case
			if isValidEndpoint(e) && ir.isValidIPAddress(e.Service()) {
				controllerIpvsDesiredChangeCount.WithLabelValues(util.Hostname, EndpointKind, UpdateAction).Inc()
				ir.upsertEndpointHandler(e)
			}
			return
		}
		if ir.isValidIPAddress(prevE.Service()) {
			controllerIpvsDesiredChangeCount.WithLabelValues(util.Hostname, EndpointKind, DeleteAction).Inc()
			ir.deleteEndpointHandler(prevE)
		}
		return
	}
	if isValidEndpoint(e) && ir.isValidIPAddress(e.Service()) {
		controllerIpvsDesiredChangeCount.WithLabelValues(util.Hostname, EndpointKind, CreateAction).Inc()
		ir.upsertEndpointHandler(e)
	}
	return
}

func (ir *IpvsReconciler) upsertEndpointHandler(e *v1alpha1.Endpoint) {
	err := withIpvsHandle(func(ipvsHandle *ipvs.Handle) error {
		iSvc := ir.ConvertServiceToIpvsService(e.Service())
		if !ipvsHandle.IsServicePresent(iSvc) {
			glog.Infof("Inserting service as could not resolve IPVS service this endpoint belongs to: %s (svc: %s)", e.ID(), e.Service().ID())
			// this case may be hit when Kubernetes Service has not been assigned a Loadbalancer IP yet when service handler was triggered
			ir.upsertServiceHandler(e.Service())
		}
		iE := ConvertEndpointToIpvsDestination(e)
		existingDests, err := ipvsHandle.GetDestinations(iSvc)
		if err != nil {
			return fmt.Errorf(
				"could not get list of destinations from IPVS service this endpoint belongs to: %s (svc: %s): %s",
				e.ID(), e.Service().ID(), err)
		}

		if existingDest := endpointInIpvsDestinations(existingDests, e); existingDest != nil {
			if hasIpvsDestinationChanged(existingDest, e) {
				err := ipvsHandle.UpdateDestination(iSvc, iE)
				if err != nil {
					return fmt.Errorf(
						"could not update existing destination from IPVS service this endpoint belongs to: %s (svc: %s): %s",
						e.ID(), e.Service().ID(), err)
				}
				controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, EndpointKind, UpdateAction).Inc()
				ir.eventRecorder.Eventf(
					e.KubeAPIV1Endpoints(), api.EventTypeNormal,
					"UpdatedIPVSDestination", "Updated IPVSDestination %s of IPVSService %s on IPVS Node %s",
					utilipvs.NewIpvsDestination(iE).ID(), utilipvs.NewIpvsService(iSvc).ID(), e.TargetRefString())
			}
		} else {
			err := ipvsHandle.NewDestination(iSvc, iE)
			if err != nil {
				return fmt.Errorf(
					"could not add new destination from IPVS service this endpoint belongs to: %s (svc: %s): %s",
					e.ID(),
					e.Service().ID(),
					err)
			}
			controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, EndpointKind, CreateAction).Inc()
			ir.eventRecorder.Eventf(
				e.KubeAPIV1Endpoints(), api.EventTypeNormal,
				"CreatedIPVSDestination", "Added IPVSDestination %s to IPVSService %s on IPVS Node %s",
				utilipvs.NewIpvsDestination(iE).ID(), utilipvs.NewIpvsService(iSvc).ID(), e.TargetRefString())
		}
		return nil
	})
	if err != nil {
		glog.Infof("error with upsertEndpointHandler: %+v", err)
	}
	return
}

func (ir *IpvsReconciler) deleteEndpointHandler(e *v1alpha1.Endpoint) {
	err := withIpvsHandle(func(ipvsHandle *ipvs.Handle) error {
		iSvc := ir.ConvertServiceToIpvsService(e.Service())
		if !ipvsHandle.IsServicePresent(iSvc) {
			return fmt.Errorf(
				"could not resolve IPVS service this endpoint belongs to: %s (svc: %s)",
				e.ID(), e.Service().ID())
		}
		iE := ConvertEndpointToIpvsDestination(e)
		existingDests, err := ipvsHandle.GetDestinations(iSvc)
		if err != nil {
			return fmt.Errorf(
				"could not get list of destinations from IPVS service this endpoint belongs to: %s (svc: %s): %s",
				e.ID(), e.Service().ID(), err)
		}
		if existingDest := endpointInIpvsDestinations(existingDests, e); existingDest != nil {
			err := ipvsHandle.DelDestination(iSvc, existingDest)
			if err != nil {
				return fmt.Errorf(
					"could not delete destinations from IPVS service this endpoint belongs to: %s (svc: %s): %s",
					e.ID(), e.Service().ID(), err)
			}
			controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, EndpointKind, DeleteAction).Inc()
			ir.eventRecorder.Eventf(
				e.KubeAPIV1Endpoints(), api.EventTypeNormal,
				"DeletedIPVSDestination", "Removed IPVSDestination %s to IPVSService %s on IPVS Node %s",
				utilipvs.NewIpvsDestination(iE).ID(), utilipvs.NewIpvsService(iSvc).ID(), e.TargetRefString())
			return nil
		}
		return fmt.Errorf(
			"could not get destination from IPVS service this endpoint belongs to: %s (svc: %s)",
			e.ID(), e.Service().ID())
	})
	if err != nil {
		glog.Infof("error with deleteEndpointHandler: %+v", err)
	}
	return
}

// CleanupOrphanedIpvs : cleans up IPVS services and destinations this controller does not track, but still belongs in its CIDR
func (ir *IpvsReconciler) CleanupOrphanedIpvs() error {
	// TODO: consider a less-greedy lock for clean up
	// this lock is for potential race-conditions with the vip interface clean up
	ir.m.Lock()
	defer ir.m.Unlock()

	err := ir.cleanupOrphanedIpvsServices()
	if err != nil {
		return err
	}
	for _, svc := range ir.sm.ListServicesWithAddress() {
		err := ir.cleanupOrphanedIpvsDestinations(svc)
		if err != nil {
			return err
		}
	}
	err = ir.cleanupOrphanedIPAddresses()
	if err != nil {
		return err
	}
	return nil
}

func (ir *IpvsReconciler) cleanupOrphanedIPAddresses() error {
	var adoptedIPAddresses []net.IP
	for _, svc := range ir.sm.ListServicesWithAddress() {
		if ir.isIPinCIDRs(*svc.Address()) {
			address := *svc.Address()
			adoptedIPAddresses = append(adoptedIPAddresses, address)
		}
	}
	vipIface, err := util.GetDummyVipInterface()
	if err != nil {
		glog.Infof("Failed getting dummy interface: %s", err)
		return err
	}
	_, err = util.DeleteNonSuppliedIPsFromInterface(adoptedIPAddresses, vipIface)
	if err != nil {
		return err
	}
	return nil
}

func (ir *IpvsReconciler) cleanupOrphanedIpvsServices() error {
	return withIpvsHandle(func(ipvsHandle *ipvs.Handle) error {
		var adoptediSvcIds = map[string]bool{}
		for _, svc := range ir.sm.ListServicesWithAddress() {
			adoptedISvc := ir.ConvertServiceToIpvsService(svc)
			adoptediSvcIds[utilipvs.NewIpvsService(adoptedISvc).ID()] = true
		}
		iSvcs, err := ipvsHandle.GetServices()
		if err != nil {
			return nil
		}
		for _, iSvc := range iSvcs {
			if _, ok := adoptediSvcIds[utilipvs.NewIpvsService(iSvc).ID()]; !ok {
				// orphaned iSvc; delete if its within the CIDR this controller manages
				if ir.isIPinCIDRs(iSvc.Address) {
					glog.Infof("Cleaning up orphaned IPVS Service: %s", utilipvs.NewIpvsService(iSvc).ID())
					err := ipvsHandle.DelService(iSvc)
					if err != nil {
						return err
					}
					controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, ServiceKind, DeleteAction).Inc()
				}
			}
		}
		return nil
	})
}

func (ir *IpvsReconciler) cleanupOrphanedIpvsDestinations(svc *v1alpha1.Service) error {
	return withIpvsHandle(func(ipvsHandle *ipvs.Handle) error {
		var adoptedIDestIds = map[string]bool{}
		for _, e := range svc.List() {
			adoptedIDest := ConvertEndpointToIpvsDestination(e)
			adoptedIDestIds[utilipvs.NewIpvsDestination(adoptedIDest).ID()] = true
		}
		iSvc := ir.ConvertServiceToIpvsService(svc)
		iDests, err := ipvsHandle.GetDestinations(iSvc)
		if err != nil {
			return nil
		}
		for _, iDest := range iDests {
			if _, ok := adoptedIDestIds[utilipvs.NewIpvsDestination(iDest).ID()]; !ok {
				// orphaned iDestination
				glog.Infof("Cleaning up orphaned IPVS Destination on IPVS Node Service: %s (iSvc: %s)",
					utilipvs.NewIpvsDestination(iDest).ID(), utilipvs.NewIpvsService(iSvc).ID())
				err := ipvsHandle.DelDestination(iSvc, iDest)
				if err != nil {
					return err
				}
				controllerIpvsProgrammedCount.WithLabelValues(util.Hostname, EndpointKind, DeleteAction).Inc()
			}
		}
		return nil
	})
}

func ConvertEndpointToIpvsDestination(endpoint *v1alpha1.Endpoint) *ipvs.Destination {
	var weight = 0
	if endpoint.Ready() {
		weight = 1
	}
	return &ipvs.Destination{
		Address:         endpoint.Address(),
		AddressFamily:   syscall.AF_INET,
		Port:            endpoint.Port(),
		Weight:          weight,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
}

func isValidEndpoint(e *v1alpha1.Endpoint) bool {
	if e.Service() != nil && e.Service().Port() == e.Port() {
		return true
	}
	return false
}

func (ir *IpvsReconciler) getIpvsScheduler(svc *v1alpha1.Service) string {
	schedStr, ok := svc.Annotations()[ipvsSchedulerAnnotation]
	if !ok {
		return ir.defaultIpvsScheduler
	}
	for _, sch := range utilipvs.SupportedIpvsSchedulers {
		if schedStr == sch {
			return sch
		}
	}
	return ir.defaultIpvsScheduler
}

func (ir *IpvsReconciler) ConvertServiceToIpvsService(svc *v1alpha1.Service) *ipvs.Service {
	return &ipvs.Service{
		Address:       *svc.Address(),
		AddressFamily: syscall.AF_INET,
		Protocol:      svc.ProtoNumber(),
		Port:          svc.Port(),
		SchedName:     ir.getIpvsScheduler(svc),
	}
}

func (ir *IpvsReconciler) hasIpvsServiceChanged(iSvc *ipvs.Service, svc *v1alpha1.Service) bool {
	if iSvc.SchedName != ir.getIpvsScheduler(svc) {
		return true
	}
	return false
}

func endpointInIpvsDestinations(dests []*ipvs.Destination, e *v1alpha1.Endpoint) *ipvs.Destination {
	for _, dest := range dests {
		if dest.Address.String() == e.Address().String() && dest.Port == e.Port() {
			return dest
		}
	}
	return nil
}

func hasIpvsDestinationChanged(iE *ipvs.Destination, e *v1alpha1.Endpoint) bool {
	var destinationReadiness = true
	if iE.Weight == 0 {
		destinationReadiness = false
	}
	if destinationReadiness != e.Ready() {
		return true
	}
	return false
}

func withIpvsHandle(f func(*ipvs.Handle) error) (err error) {
	ipvsHandle, err := ipvs.New("")
	if err != nil {
		return err
	}
	defer ipvsHandle.Close()

	return f(ipvsHandle)
}
