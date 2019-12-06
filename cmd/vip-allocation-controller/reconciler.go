package main

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	"k8s.io/client-go/tools/record"

	"github.com/Demonware/balanced/pkg/util"
	api "k8s.io/api/core/v1"
	coreinformer "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
)

const (
	ipPoolAnnotation    = "balanced.demonware.net/ip-pool"
	ipRequestAnnotation = "balanced.demonware.net/ip-request"
)

type VIPReconciler struct {
	client          clientset.Interface
	ipPoolStr       *string
	cidr            *net.IPNet
	cidrIps         []string
	serviceInformer cache.SharedIndexInformer
	eventRecorder   record.EventRecorder
	sm              v1alpha1.ServiceManager
}

func NewVIPReconciler(client clientset.Interface, ipPoolName *string, cidr *net.IPNet, s coreinformer.ServiceInformer,
	eventRecorder record.EventRecorder, sm v1alpha1.ServiceManager) *VIPReconciler {

	// initl pseudo random generator
	rand.Seed(time.Now().Unix())

	if ipPoolName != nil {
		glog.Infof("Initializing VIPReconciler for IP Pool: %s (%s)", *ipPoolName, cidr.String())
	}

	return &VIPReconciler{
		client:          client,
		ipPoolStr:       ipPoolName,
		cidr:            cidr,
		serviceInformer: s.Informer(),
		eventRecorder:   eventRecorder,
		sm:              sm,
	}
}

func (v *VIPReconciler) ipPoolName() string {
	if v.ipPoolStr == nil {
		return ""
	}
	return *v.ipPoolStr
}

func (v *VIPReconciler) isSelectedPool(svc *v1alpha1.Service) bool {
	annotations := svc.Annotations()
	val := annotations[ipPoolAnnotation]
	return v.ipPoolName() == val || v.cidr.String() == val
}

// requestedIPs returns a slice of valid IP addresses specified by the user through the use of an annotation
func requestedIPs(svc *v1alpha1.Service) (ips []string, err error) {
	annotations := svc.Annotations()
	val := annotations[ipRequestAnnotation]
	rawIPStrs := strings.Split(val, ",")
	for _, rawIPStr := range rawIPStrs {
		ip := net.ParseIP(strings.TrimSpace(rawIPStr))
		if ip == nil {
			return nil, fmt.Errorf("error parsing IPs from annotation, potential invalid ips?")
		}
		ips = append(ips, ip.String())
	}
	return
}

func (v *VIPReconciler) AssignVIPIfValidService(prevSvc *v1alpha1.Service, svc *v1alpha1.Service) {
	// TODO: not currently thread-safe, implement thread-safety before turning on concurrency on this controller
	// race conditions could happen where the same free IP can be handed out by v.freeIPs()

	// we ignore if it's a deletion event, we know because svc would be nil
	if prevSvc != nil && svc == nil {
		return
	}
	// if Service already has LoadBalancerIP, then ignore
	if svc.Address() != nil {
		glog.V(4).Infof("Service already has a LoadBalancer IP Address assigned: %s", svc.Address().String())
		return
	}
	if !v.isSelectedPool(svc) {
		glog.V(2).Infof("Service is ignored by this VIP controller because it does not match the designated IP Pool: %s", svc.ID())
		return
	}
	var (
		assignedIP string
		err        error
	)
	reqIPs, err := requestedIPs(svc)
	if err == nil && len(reqIPs) > 0 {
		assignedIP, err = acquireFromSpecifiedIPs(svc, reqIPs, v.FreeIPs())
		if err != nil {
			v.eventRecorder.Event(
				svc.KubeAPIV1Service(), api.EventTypeWarning, "IPRequestCriteriaNotMet", err.Error())
			return
		}
	} else {
		assignedIP, err = v.randomFreeIP()
		if err != nil {
			v.eventRecorder.Event(
				svc.KubeAPIV1Service(), api.EventTypeWarning, "OutOfLoadBalancerIP", err.Error())
			return
		}
	}
	if assignedIP != "" {
		err = v.allocateLoadbalancerIP(svc, assignedIP)
		if err != nil {
			glog.Infof("Error Assigning VIP to Valid Service: %v, err: %s", svc, err)
			return
		}
		return
	}
	v.eventRecorder.Event(
		svc.KubeAPIV1Service(), api.EventTypeWarning, "NoSuitableLoadBalancerIP", "No suitable IP was found for this Service.")
}

// acquireFromSpecifiedIP attempts to acquire from the list of user-specified IP addresses from the pool for allocation
// the first IP from the list that meets the criteria will be returned as a string
// ie. the IP is free and is contained within the CIDR range of this controller's jurisdiction
func acquireFromSpecifiedIPs(svc *v1alpha1.Service, specIPs []string, freeIPs []string) (string, error) {
	for _, specIP := range specIPs {
		for _, freeIP := range freeIPs {
			if specIP == freeIP {
				return specIP, nil
			}
		}
	}
	return "", fmt.Errorf("No free IPs were found specified from the list of user-specified IPs")
}

func (v *VIPReconciler) allocateLoadbalancerIP(svc *v1alpha1.Service, ipAddr string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		item, exists, err := v.serviceInformer.GetIndexer().GetByKey(svc.KubeAPIV1ServiceName())
		if !exists || err != nil {
			glog.Infof("Service no longer exists when allocating IP, possible race condition?: %s", svc.KubeAPIV1ServiceName())
			return nil
		}
		kubeSvc := item.(*api.Service)
		if len(kubeSvc.Status.LoadBalancer.Ingress) != 0 {
			glog.V(3).Infof("Loadbalancer IP has already been assigned. Skipping.")
			return nil
		}
		v.eventRecorder.Event(svc.KubeAPIV1Service(), api.EventTypeNormal, "AllocatingLoadBalancerIP", "Allocating a LoadBalancer IP")
		kubeSvc.Status.LoadBalancer = api.LoadBalancerStatus{
			Ingress: []api.LoadBalancerIngress{
				{IP: ipAddr},
			},
		}
		_, err = v.client.CoreV1().Services(kubeSvc.Namespace).UpdateStatus(kubeSvc)
		v.eventRecorder.Event(svc.KubeAPIV1Service(), api.EventTypeNormal, "AllocatedLoadBalancerIP", fmt.Sprintf("Allocated a LoadBalancer IP/ExternalIP: %s", ipAddr))
		return err
	})
}

func (v *VIPReconciler) existingIPs() []string {
	var existingIPs []string
	for _, svc := range v.sm.List() {
		address := svc.Address()
		if address != nil && v.cidr.Contains(*address) {
			existingIPs = append(existingIPs, address.String())
		}
	}
	// could have duplicated IPs here
	return existingIPs
}

// allIPs return a slice of IP strings within the CIDR
func (v *VIPReconciler) allIPs() []string {
	// if already computed, just return stored slice
	if v.cidrIps != nil {
		return v.cidrIps
	}

	var ips []string
	ip := v.cidr.IP
	for ip := ip.Mask(v.cidr.Mask); v.cidr.Contains(ip); inc(ip) {
		ips = append(ips, ip.String())
	}
	// remove network address and broadcast address
	if len(ips) > 2 {
		v.cidrIps = ips[1 : len(ips)-1]
		return v.cidrIps
	}
	// just a /32
	v.cidrIps = ips
	return v.cidrIps
}

//  http://play.golang.org/p/m8TNTtygK0
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func (v *VIPReconciler) RefreshMetrics() error {
	existingIPs := v.existingIPs()
	freeIps := v.FreeIPs()

	VipUsed.WithLabelValues(v.ipPoolName(), v.cidr.String()).Set(float64(len(existingIPs)))
	VipFree.WithLabelValues(v.ipPoolName(), v.cidr.String()).Set(float64(len(freeIps)))
	return nil
}

func (v *VIPReconciler) FreeIPs() []string {
	var (
		allIps      []string
		existingIps []string
		freeIps     []string
	)

	allIps = v.allIPs()
	existingIps = v.existingIPs()
	freeIps = util.Difference(allIps, existingIps)

	return freeIps
}

func (v *VIPReconciler) randomFreeIP() (string, error) {
	// TODO: add a thread-safe cache/map to keep track of IP address offers
	// TODO: ignore as a freeIP if it was offered recently by this function and is still in the computed free pool
	var freeIps []string
	freeIps = v.FreeIPs()
	if len(freeIps) < 1 {
		return "", fmt.Errorf("Cannot allocate a LoadBalancer IP - out of IPs from CIDR: %s", v.cidr.String())
	}

	return freeIps[rand.Intn(len(freeIps))], nil
}
