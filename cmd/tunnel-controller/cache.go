package main

import (
	"net"
	"time"

	"github.com/patrickmn/go-cache"
)

type CachedHostPod struct {
	HostPodID   string
	ContainerID string
	Addresses   []net.IP
}

type HostPodCache interface {
	Delete(h *HostPod)
	Set(h *HostPod)
	HasChanged(h *HostPod) bool
}

type ThreadsafeHostPodCache struct {
	cache *cache.Cache
}

func NewThreadsafeHostPodCache() *ThreadsafeHostPodCache {
	return &ThreadsafeHostPodCache{
		// thread-safe implementation
		// TODO: make it configurable
		cache: cache.New(24*time.Hour, 24*time.Hour),
	}
}

func (hpc *ThreadsafeHostPodCache) Delete(h *HostPod) {
	hpc.cache.Delete(h.ID())
}

func (hpc *ThreadsafeHostPodCache) Set(h *HostPod) {
	var cachedHostPod *CachedHostPod
	cachedItem, ok := hpc.cache.Get(h.ID())
	if !ok {
		cachedHostPod = &CachedHostPod{}
	} else {
		cachedHostPod = cachedItem.(*CachedHostPod)
	}
	cachedHostPod.HostPodID = h.ID()
	cachedHostPod.ContainerID = containerIDString(h.ContainerID())
	cachedHostPod.Addresses = h.Addresses()

	hpc.cache.Set(h.ID(), cachedHostPod, cache.DefaultExpiration)
}

func (hpc *ThreadsafeHostPodCache) HasChanged(h *HostPod) bool {
	cachedItem, ok := hpc.cache.Get(h.ID())
	if !ok {
		return true
	}
	cachedHostPod := cachedItem.(*CachedHostPod)
	if cachedHostPod.ContainerID == containerIDString(h.ContainerID()) {
		cachedAddresses := convertNetIPToStringSlice(cachedHostPod.Addresses)
		addresses := convertNetIPToStringSlice(h.Addresses())
		return !hasIdenticalElements(cachedAddresses, addresses)
	}
	return true
}

func containerIDString(containerID *string) string {
	var containerIDStr string
	if containerID != nil {
		containerIDStr = *containerID
	}
	return containerIDStr
}

// hasIdenticalElements: returns true if all elements in x are in y and vice versa, without comparing order
func hasIdenticalElements(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	m := make(map[string]bool)
	for _, s := range x {
		m[s] = true
	}
	for _, s := range y {
		if _, ok := m[s]; !ok {
			return false
		}
	}
	return true
}

func convertNetIPToStringSlice(ips []net.IP) (s []string) {
	for _, ip := range ips {
		s = append(s, ip.String())
	}
	return
}
