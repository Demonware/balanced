package main

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/Demonware/balanced/pkg/util"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	api "k8s.io/api/core/v1"
)

const (
	legacyIgnoreAnnotation = "ipvs-tunnel-controller.demonware.net/ignore"
	ignoreAnnotation       = "balanced.demonware.net/tunnel-ignore"
)

type HostPod struct {
	pod *api.Pod
	sm  v1alpha1.ServiceManager
	m   sync.RWMutex
}

func NewHostPod(pod *api.Pod) *HostPod {
	namedLogger := util.NewNamedLogger(
		fmt.Sprintf("HostPodServiceManager-%s/%s", pod.Namespace, pod.Name),
		3)
	return &HostPod{
		pod: pod,
		sm:  v1alpha1.NewMapServiceManager(namedLogger),
	}
}

func (h *HostPod) ID() string {
	return fmt.Sprintf("%s/%s", h.pod.Namespace, h.pod.Name)
}

func (h *HostPod) UpdatePod(pod *api.Pod) error {
	h.m.Lock()
	defer h.m.Unlock()

	if pod.Namespace+"/"+pod.Name != h.ID() {
		return fmt.Errorf("Pod does not match HostPod ID")
	}
	h.pod = pod
	return nil
}

func (h *HostPod) Set(svc *v1alpha1.Service) error {
	if h.sm.Exists(svc) {
		_, _, err := h.sm.Update(svc)
		if err != nil {
			return err
		}
		return nil
	}
	err := h.sm.Add(svc)
	if err != nil {
		return err
	}
	return nil
}

func (h *HostPod) Remove(svc *v1alpha1.Service) error {
	return h.sm.Remove(svc)
}

func (h *HostPod) Addresses() (addrs []net.IP) {
	for _, svc := range h.sm.ListServicesWithAddress() {
		addrRef := svc.Address()
		addrs = append(addrs, *addrRef)
	}
	return
}

func (h *HostPod) IsValidPod() bool {
	h.m.RLock()
	defer h.m.RUnlock()

	if h.pod.Spec.HostNetwork {
		return false
	}
	annotations := h.pod.GetAnnotations()
	if _, exists := annotations[ignoreAnnotation]; exists {
		return false
	}
	if _, exists := annotations[legacyIgnoreAnnotation]; exists {
		return false
	}
	return true
}

// ContainerID: find first running container ID from normal containers and fallback to initContainers
func (h *HostPod) ContainerID() *string {
	containerID := firstRunningContainerID(h.pod.Status.ContainerStatuses)
	if containerID != nil {
		return containerID
	}
	return firstRunningContainerID(h.pod.Status.InitContainerStatuses)
}

func (h *HostPod) Pod() *api.Pod {
	return h.pod
}

func firstRunningContainerID(containerStatuses []api.ContainerStatus) *string {
	if containerStatuses == nil {
		return nil
	}
	for _, conStatus := range containerStatuses {
		if conStatus.State.Running == nil {
			continue
		}
		containerID := strings.TrimSpace(conStatus.ContainerID)
		if containerID == "" {
			continue
		}
		return &containerID
	}
	return nil
}
