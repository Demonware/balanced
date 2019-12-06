package health

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
)

type HealthCheck interface {
	Healthy() bool
}

type GenericHealthCheck struct {
	Name          string        `json:"name"`
	Period        time.Duration `json:"period_ns"`
	LastHeartBeat time.Time     `json:"last_heartbeat,omitempty"`

	m sync.Mutex
}

func NewGenericHealthCheck(name string, period time.Duration) *GenericHealthCheck {
	return &GenericHealthCheck{
		Name:   name,
		Period: period,
	}
}

func (hc *GenericHealthCheck) String() string {
	return fmt.Sprintf("GenericHealthCheck<%s>", hc.Name)
}

func (hc *GenericHealthCheck) HeartBeat() {
	hc.m.Lock()
	defer hc.m.Unlock()
	hc.LastHeartBeat = time.Now()
	glog.V(4).Infof("Heartbeat Registered: %s @ %s", hc, hc.LastHeartBeat)
}

func (hc *GenericHealthCheck) Healthy() bool {
	hc.m.Lock()
	defer hc.m.Unlock()
	if time.Since(hc.LastHeartBeat) > hc.Period {
		return false
	}
	return true
}
