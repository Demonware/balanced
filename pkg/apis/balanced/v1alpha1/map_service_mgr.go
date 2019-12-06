package v1alpha1

import (
	"fmt"

	"github.com/sasha-s/go-deadlock"

	"github.com/Demonware/balanced/pkg/util"
	"k8s.io/client-go/tools/cache"
)

type ServiceOnChangeHandler func(prevSvc *Service, updatedSvc *Service)

type MapServiceManager struct {
	logger util.Logger
	svcs   map[string]*Service
	m      deadlock.Mutex
}

func NewMapServiceManager(logger util.Logger) *MapServiceManager {
	return &MapServiceManager{
		logger: logger,
		svcs:   make(map[string]*Service),
	}
}

func (sm *MapServiceManager) Exists(svc *Service) bool {
	sm.m.Lock()
	defer sm.m.Unlock()
	return sm.exists(svc)
}

func (sm *MapServiceManager) exists(svc *Service) bool {
	_, ok := sm.svcs[svc.ID()]
	return ok
}

// ResolveByKubeMetaNamespaceKey : finds all Services with the same Kubernetes Service Object Key
func (sm *MapServiceManager) ResolveByKubeMetaNamespaceKey(key string) (svcs []*Service, err error) {
	// TODO: implement a tree based service manager to make search faster?
	sm.with(func() {
		for _, svc := range sm.svcs {
			var objKey string
			objKey, err = cache.MetaNamespaceKeyFunc(svc.kubeService)
			if err != nil {
				return
			}
			if objKey == key {
				svcs = append(svcs, svc)
			}
		}
		return
	})
	return
}

func noLongerExistentServiceKeys(currentServices []*Service, existingServices []*Service) (diffServiceKeys []string) {
	var (
		existingServiceKeys []string
		currentServiceKeys  []string
	)
	for _, existingSvc := range existingServices {
		existingServiceKeys = append(existingServiceKeys, existingSvc.ID())
	}
	for _, svc := range currentServices {
		currentServiceKeys = append(currentServiceKeys, svc.ID())
	}
	diffServiceKeys = util.Difference(existingServiceKeys, currentServiceKeys)
	return
}

// RemoveExistingServicesNoLongerCurrent removes existing services that are managed by the MapServiceManager that are no longer in existence in the real world
func (sm *MapServiceManager) RemoveExistingServicesNoLongerCurrent(currentServices []*Service, existingServices []*Service, handlers ...ServiceOnChangeHandler) (err error) {
	diffServiceKeys := noLongerExistentServiceKeys(currentServices, existingServices)
	for _, key := range diffServiceKeys {
		err = sm.RemoveByID(key, handlers...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sm *MapServiceManager) Get(svcID string) (svc *Service, err error) {
	sm.with(func() {
		svc, err = sm.get(svcID)
	})
	return
}

func (sm *MapServiceManager) get(svcID string) (*Service, error) {
	var svc *Service
	svc, ok := sm.svcs[svcID]
	if !ok {
		return nil, fmt.Errorf("Cannot get Service, svcID does not exist: %s", svcID)
	}
	return svc, nil
}

func (sm *MapServiceManager) List() (list []*Service) {
	sm.with(func() {
		list = sm.list()
	})
	return
}

func (sm *MapServiceManager) list() []*Service {
	var svcs []*Service
	for _, svc := range sm.svcs {
		svcs = append(svcs, svc)
	}
	return svcs
}

func (sm *MapServiceManager) ListServicesWithAddress() (list []*Service) {
	sm.with(func() {
		list = sm.listServicesWithAddress()
	})
	return
}

func (sm *MapServiceManager) listServicesWithAddress() []*Service {
	var svcs []*Service
	for _, svc := range sm.svcs {
		if svc.Address() != nil {
			svcs = append(svcs, svc)
		}
	}
	return svcs
}

func (sm *MapServiceManager) Add(svc *Service, handlers ...ServiceOnChangeHandler) (err error) {
	sm.with(func() {
		err = sm.add(svc)
	})
	if err != nil {
		return err
	}
	for _, handle := range handlers {
		handle(nil, svc)
	}
	return nil
}

func (sm *MapServiceManager) add(svc *Service) error {
	if sm.exists(svc) {
		return fmt.Errorf("Cannot add a Service that already exists: %s", svc.ID())
	}
	sm.logger.Infof("Adding service to MapServiceManager: %s", svc.ID())
	sm.svcs[svc.ID()] = svc
	return nil
}

func (sm *MapServiceManager) Update(svc *Service, handlers ...ServiceOnChangeHandler) (updated bool, prevSvc *Service, err error) {
	sm.with(func() {
		updated, prevSvc, err = sm.update(svc)
	})
	if err != nil {
		return updated, prevSvc, err
	}
	if !updated {
		svc = prevSvc
	}
	for _, handle := range handlers {
		handle(prevSvc, svc)
	}
	return updated, prevSvc, nil
}

func (sm *MapServiceManager) update(svc *Service) (updated bool, prevSvc *Service, err error) {
	if !sm.exists(svc) {
		return false, nil, fmt.Errorf("Cannot update a Service that does not exist: %s", svc.ID())
	}
	prevSvc = sm.svcs[svc.ID()]
	if prevSvc.Equals(svc) {
		sm.logger.Infof("Service in MapServiceManager has not been updated, no-op update: %s", svc.ID())
		return false, prevSvc, nil
	}
	_, err = svc.UpsertEndpoints(prevSvc.List())
	if err != nil {
		return false, nil, err
	}
	sm.logger.Infof("Updating service in MapServiceManager: %s", svc.ID())
	sm.svcs[svc.ID()] = svc
	return true, prevSvc, nil
}

func (sm *MapServiceManager) Remove(svc *Service, handlers ...ServiceOnChangeHandler) (err error) {
	sm.with(func() {
		err = sm.remove(svc)
	})
	if err != nil {
		return err
	}
	for _, handle := range handlers {
		handle(svc, nil)
	}
	return nil
}

func (sm *MapServiceManager) remove(svc *Service) error {
	if !sm.exists(svc) {
		return fmt.Errorf("Cannot remove a Service that does not exist: %s", svc.ID())
	}
	sm.logger.Infof("Deleting service from MapServiceManager: %s", svc.ID())
	delete(sm.svcs, svc.ID())
	return nil
}

func (sm *MapServiceManager) RemoveByID(svcID string, handlers ...ServiceOnChangeHandler) (err error) {
	var svc *Service
	sm.with(func() {
		svc, err = sm.get(svcID)
		if err != nil {
			return
		}
		err = sm.remove(svc)
		if err != nil {
			return
		}
	})
	if err != nil {
		return
	}
	for _, handle := range handlers {
		handle(svc, nil)
	}
	return
}

// RemoveServicesRecursively remove Endpoints of a Service first before removal
func (sm *MapServiceManager) RemoveServicesRecursively(svcs []*Service, svcOnChangeHandler []ServiceOnChangeHandler, eOnChangeHandler []EndpointOnChangeHandler) error {
	for _, svc := range svcs {
		for _, endpoint := range svc.List() {
			err := svc.Remove(endpoint, eOnChangeHandler...)
			if err != nil {
				return err
			}
		}
		err := sm.Remove(svc, svcOnChangeHandler...)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateServicesRecursively will add Service to SM if not already exist and its Endpoints,
// otherwise, it will update the existing Service in the SM along with its Endpoints
func (sm *MapServiceManager) UpdateServicesRecursively(svcs []*Service, endpoints []*Endpoint, svcOnChangeHandler []ServiceOnChangeHandler, eOnChangeHandler []EndpointOnChangeHandler) error {
	for _, svc := range svcs {
		if err := sm.Add(svc, svcOnChangeHandler...); err == nil {
			// parent Endpoints to Services
			_, err = svc.UpsertEndpoints(endpoints, eOnChangeHandler...)
			if err != nil {
				return err
			}
		} else {
			updated, prevSvc, err := sm.Update(svc, svcOnChangeHandler...)
			if err != nil {
				return err
			}
			if !updated {
				svc = prevSvc
			}
			_, err = svc.UpdateEndpoints(endpoints, eOnChangeHandler...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (sm *MapServiceManager) with(f func()) {
	sm.m.Lock()
	defer sm.m.Unlock()
	f()
}
