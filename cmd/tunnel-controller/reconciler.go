package main

import (
	"fmt"

	"github.com/sasha-s/go-deadlock"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"

	"github.com/golang/glog"
	api "k8s.io/api/core/v1"
	coreinformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type TunnelReconciler struct {
	hostname           string
	podInformer        cache.SharedIndexInformer
	serviceManager     v1alpha1.ServiceManager
	tunnelConfigurator TunnelConfigurator
	hostPodCache       HostPodCache

	hostPods map[string]*HostPod
	m        deadlock.Mutex
}

func NewTunnelReconciler(
	hostname string,
	podInformer coreinformer.PodInformer,
	serviceManager v1alpha1.ServiceManager,
	tunnelConfigurator TunnelConfigurator,
	hostPodCache HostPodCache,
) *TunnelReconciler {

	return &TunnelReconciler{
		hostname:           hostname,
		podInformer:        podInformer.Informer(),
		serviceManager:     serviceManager,
		tunnelConfigurator: tunnelConfigurator,

		hostPods:     make(map[string]*HostPod),
		hostPodCache: hostPodCache,
	}
}

func (t *TunnelReconciler) OnChangeEndpointHandler(prevE *v1alpha1.Endpoint, e *v1alpha1.Endpoint) {
	glog.V(4).Infof("TunnelReconciler.OnChangeEndpointHandler called")
	if prevE != nil {
		if e != nil {
			if isValidEndpoint(t.hostname, e) {
				t.upsertEndpointHandler(e) // update case
			}
		} else if isValidEndpoint(t.hostname, prevE) {
			t.deleteEndpointHandler(prevE) // delete case
		}
	} else if isValidEndpoint(t.hostname, e) {
		t.upsertEndpointHandler(e) // insert case
	}
}

func isValidEndpoint(hostname string, e *v1alpha1.Endpoint) bool {
	if e.IsTargetPod() && e.NodeName() != nil && *e.NodeName() == hostname {
		if e.Service() != nil && e.Service().Port() == e.Port() {
			return true
		}
		return false
	}
	return false
}

func (t *TunnelReconciler) podFromEndpoint(e *v1alpha1.Endpoint) (*api.Pod, error) {
	podName := fmt.Sprintf("%s/%s", e.TargetRef().Namespace, e.TargetRef().Name)
	obj, exists, err := t.podInformer.GetIndexer().GetByKey(podName)
	if err != nil || !exists {
		return nil, fmt.Errorf("could not locate pod object in PodInformer: %s (err: %s)", podName, err)
	}
	pod := obj.(*api.Pod)
	return pod, nil
}

// getHostPodFromPod: side effect of adding to hostPods map if not exists
func (t *TunnelReconciler) getHostPodFromPod(pod *api.Pod) *HostPod {
	t.m.Lock()
	defer t.m.Unlock()

	hostPod, exists := t.hostPods[pod.Namespace+"/"+pod.Name]
	if !exists {
		hostPod = NewHostPod(pod)
		t.hostPods[hostPod.ID()] = hostPod
		return hostPod
	}
	err := hostPod.UpdatePod(pod)
	if err != nil {
		glog.Infof("cannot update Pod in hostPod: %s", err)
	}
	return hostPod
}

func (t *TunnelReconciler) upsertEndpointHandler(e *v1alpha1.Endpoint) {
	pod, err := t.podFromEndpoint(e)
	if err != nil {
		glog.Infof("%s", err)
		return
	}
	hostPod := t.getHostPodFromPod(pod)
	err = hostPod.Set(e.Service())
	if err != nil {
		glog.Infof("cannot upsert Endpoint's Service into hostPod: %s", err)
	}
	return
}

func (t *TunnelReconciler) deleteEndpointHandler(e *v1alpha1.Endpoint) {
	if e.Service() == nil {
		return
	}
	pod, err := t.podFromEndpoint(e)
	if err != nil {
		glog.Infof("%s", err)
		return
	}
	hostPod := t.getHostPodFromPod(pod)
	err = hostPod.Remove(e.Service())
	if err != nil {
		glog.V(3).Infof("warning: cannot remove Endpoint's Service from hostPod, probably already removed: %s %s", hostPod.ID(), err)
	}
	return
}

func (t *TunnelReconciler) Sync() error {
	t.updateHostPods()
	err := t.syncTunnelEndpoints()
	if err != nil {
		return err
	}
	return nil
}

func (t *TunnelReconciler) updateHostPods() {
	var podNamesExisting = make(map[string]bool)
	pods := t.podInformer.GetStore().List()
	for _, obj := range pods {
		pod := obj.(*api.Pod)
		if pod.Spec.NodeName != t.hostname {
			continue
		}
		podNamesExisting[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = true
		// creates host pod if it doesn't exist
		_ = t.getHostPodFromPod(pod)
	}
	// remove pods that no longer exist on host
	t.m.Lock()
	defer t.m.Unlock()
	for _, hostPod := range t.hostPods {
		if _, exists := podNamesExisting[hostPod.ID()]; !exists {
			delete(t.hostPods, hostPod.ID())
			t.hostPodCache.Delete(hostPod)
		}
	}
	return
}

func (t *TunnelReconciler) syncTunnelEndpoints() error {
	t.m.Lock()
	defer t.m.Unlock()

	for _, hostPod := range t.hostPods {
		if hostPod.IsValidPod() {
			if t.hostPodCache.HasChanged(hostPod) && hostPod.ContainerID() != nil {
				err := t.tunnelConfigurator.configure(hostPod)
				if err != nil {
					glog.Errorf("tunnelConfigurator error: %s", err)
					// do not return as an error because one pod/container is having issues
					continue
				}
				t.hostPodCache.Set(hostPod)
			}
		}
	}
	return nil
}
