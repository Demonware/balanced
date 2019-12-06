package v1alpha1

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"syscall"

	"github.com/Demonware/balanced/pkg/util"

	"github.com/golang/glog"
	deadlock "github.com/sasha-s/go-deadlock"
	api "k8s.io/api/core/v1"
)

type EndpointOnChangeHandler func(prevE *Endpoint, updatedE *Endpoint)

var ProtoNameToNumberMap = map[api.Protocol]uint16{
	api.ProtocolTCP: syscall.IPPROTO_TCP,
	api.ProtocolUDP: syscall.IPPROTO_UDP,
}

type Service struct {
	ip          *net.IP
	port        api.ServicePort
	protocol    api.Protocol
	kubeService *api.Service

	endpoints map[string]*Endpoint
	m         deadlock.Mutex
}

type ServiceOption func(*Service) error

type ServiceUpdateRecord struct {
	Added   []*Endpoint
	Updated []*Endpoint
	Removed []*Endpoint
	Ignored []*Endpoint
}

func getLoadBalancerIP(kubeService *api.Service) (ip *net.IP) {
	for _, ingressObj := range kubeService.Status.LoadBalancer.Ingress {
		if ingressObj.IP != "" {
			addr := net.ParseIP(ingressObj.IP)
			ip = &addr
			return
		}
	}
	return
}

// ConvertKubeAPIV1ServiceToServices assumes a kubeService with Service Type: LoadBalancer
func ConvertKubeAPIV1ServiceToServices(kubeService *api.Service, opts ...ServiceOption) ([]*Service, error) {
	var svcs []*Service
	for _, portSpec := range kubeService.Spec.Ports {
		var svc *Service
		svc, err := NewService(kubeService, portSpec, opts...)
		if err != nil {
			return nil, err
		}
		svcs = append(svcs, svc)
	}
	return svcs, nil
}

func NewService(kubeService *api.Service, portSpec api.ServicePort, opts ...ServiceOption) (*Service, error) {
	s := &Service{
		ip:          getLoadBalancerIP(kubeService),
		port:        portSpec,
		protocol:    portSpec.Protocol,
		kubeService: kubeService,
		endpoints:   make(map[string]*Endpoint),
	}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// TargetPort resolves target port using endpoint information
func (s *Service) TargetPort() *uint16 {
	for _, endpoint := range s.endpoints {
		port := endpoint.Port()
		return &port
	}
	return nil
}

func (s *Service) ID() string {
	return fmt.Sprintf(
		"%s/%s/%d",
		s.KubeAPIV1ServiceName(),
		s.Protocol(),
		s.Port())
}

func (s *Service) KubeAPIV1ServiceName() string {
	return s.kubeService.GetNamespace() + "/" + s.kubeService.GetName()
}

func (s *Service) KubeAPIV1Service() *api.Service {
	return s.kubeService
}

func (s *Service) Address() *net.IP {
	return s.ip
}

func (s *Service) Port() uint16 {
	return uint16(s.port.Port)
}

func (s *Service) PortName() string {
	return s.port.Name
}

func (s *Service) Protocol() api.Protocol {
	return api.Protocol(strings.ToUpper(string(s.protocol)))
}

func (s *Service) ProtoNumber() uint16 {
	return ProtoNameToNumberMap[s.Protocol()]
}

func (s *Service) isEqualAddress(svc *Service) bool {
	if s.Address() == nil && svc.Address() != nil {
		return false
	} else if s.Address() != nil && svc.Address() == nil {
		return false
	} else if s.Address() == svc.Address() {
		return true
	}
	return s.Address() != nil && svc.Address() != nil && s.Address().String() == svc.Address().String()
}

func (s *Service) Equals(svc *Service) bool {
	equalSvcNamePortProtocol := s.ID() == svc.ID() // the ID can determine that
	equalServiceAnnotations := reflect.DeepEqual(s.KubeAPIV1Service().Annotations, svc.KubeAPIV1Service().Annotations)

	return equalSvcNamePortProtocol && equalServiceAnnotations && s.isEqualAddress(svc)
}

func (s *Service) Annotations() map[string]string {
	return s.kubeService.Annotations
}

func (s *Service) IsChildEndpoint(e *Endpoint) bool {
	isSameName := e.KubeAPIV1EndpointsName() == s.KubeAPIV1ServiceName()
	isSamePort := e.PortName() == s.PortName()
	// do not compare port number as it doesn't have to be same to be considered its endpoint
	// ipvs-controller does further enforcements of port number
	if isSameName && isSamePort {
		return true
	}
	return false
}

func (s *Service) Exists(e *Endpoint) (exists bool) {
	s.with(func() {
		exists = s.exists(e)
	})
	return
}

func (s *Service) exists(e *Endpoint) bool {
	_, ok := s.endpoints[e.ID()]
	return ok
}

func (s *Service) Get(endpointID string) (e *Endpoint, err error) {
	s.with(func() {
		e, err = s.get(endpointID)
	})
	return
}

func (s *Service) get(endpointID string) (*Endpoint, error) {
	var endpoint *Endpoint
	endpoint, ok := s.endpoints[endpointID]
	if !ok {
		return nil, fmt.Errorf("Cannot get Endpoint, as endpointID does not exist: %s", endpointID)
	}
	return endpoint, nil
}

func (s *Service) List() (endpoints []*Endpoint) {
	s.with(func() {
		endpoints = s.list()
	})
	return
}

func (s *Service) list() []*Endpoint {
	var endpoints []*Endpoint
	for _, endpoint := range s.endpoints {
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

func (s *Service) Add(e *Endpoint, handlers ...EndpointOnChangeHandler) (err error) {
	s.with(func() {
		err = s.add(e)
	})
	if err != nil {
		return err
	}
	for _, handle := range handlers {
		handle(nil, e)
	}
	return nil
}

func (s *Service) add(e *Endpoint) error {
	if s.exists(e) {
		return fmt.Errorf("Cannot add an Endpoint that already exists: %s", e.ID())
	}
	s.endpoints[e.ID()] = e

	// set service parent
	e.SetService(s)
	return nil
}

func (s *Service) Update(e *Endpoint, handlers ...EndpointOnChangeHandler) (err error) {
	var (
		updated bool
		prevE   *Endpoint
	)
	s.with(func() {
		updated, prevE, err = s.update(e)
	})
	if err != nil {
		return err
	}
	if !updated {
		e = prevE
	}
	for _, handle := range handlers {
		handle(prevE, e)
	}
	return nil
}

func (s *Service) update(e *Endpoint) (updated bool, prevE *Endpoint, err error) {
	if !s.exists(e) {
		return false, nil, fmt.Errorf("Cannot update an Endpoint that does not exist: %s", e.ID())
	}

	prevE = s.endpoints[e.ID()]
	if prevE.Equals(e) {
		glog.V(4).Infof("%s: Endpoint in Service has not been updated, no-op update: %s", s.ID(), e.ID())
		return false, prevE, nil
	}
	glog.V(3).Infof("%s: Updating endpoint: %s", s.ID(), e.ID())
	s.endpoints[e.ID()] = e

	// set service parent
	e.SetService(s)
	return true, prevE, nil
}

func (s *Service) Remove(e *Endpoint, handlers ...EndpointOnChangeHandler) (err error) {
	s.with(func() {
		err = s.remove(e)
	})
	if err != nil {
		return err
	}
	for _, handle := range handlers {
		handle(e, nil)
	}
	return nil
}

func (s *Service) remove(e *Endpoint) error {
	if !s.exists(e) {
		return fmt.Errorf("Cannot remove an Endpoint that does not exist: %s", e.ID())
	}
	glog.V(3).Infof("%s: Removing endpoint: %s", s.ID(), e.ID())
	delete(s.endpoints, e.ID())
	return nil
}

// UpsertEndpoints : will only insert or update endpoints that are a child of this Service, ignore the rest
func (s *Service) UpsertEndpoints(endpoints []*Endpoint, handlers ...EndpointOnChangeHandler) (result *ServiceUpdateRecord, err error) {
	result = &ServiceUpdateRecord{}
	for _, e := range endpoints {
		if s.IsChildEndpoint(e) {
			err = s.Add(e, handlers...)
			if err != nil {
				err = s.Update(e, handlers...)
				// this will not hit since the only error check is of e's existence which the add already has done
				// unless another goroutine has removed the endpoint in the middle of this func
				if err != nil {
					return nil, err
				}
				result.Updated = append(result.Updated, e)
				continue
			}
			result.Added = append(result.Added, e)
			continue
		}
		result.Ignored = append(result.Ignored, e)
	}
	return result, nil
}

// UpdateEndpoints : will insert or update endpoints similarly to UpsertEndpoints, but will remove children that do not exist in endpoints slice
func (s *Service) UpdateEndpoints(endpoints []*Endpoint, handlers ...EndpointOnChangeHandler) (result *ServiceUpdateRecord, err error) {
	err = s.deleteEndpointsNotSupplied(endpoints, handlers...)
	if err != nil {
		return nil, err
	}
	result, err = s.UpsertEndpoints(endpoints, handlers...)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Service) deleteEndpointsNotSupplied(endpoints []*Endpoint, handlers ...EndpointOnChangeHandler) error {

	var (
		existingIds    []string
		specifiedIds   []string
		nonExistentIds []string
	)
	for _, e := range s.List() {
		existingIds = append(existingIds, e.ID())
	}
	for _, e := range endpoints {
		specifiedIds = append(specifiedIds, e.ID())
	}
	nonExistentIds = util.Difference(existingIds, specifiedIds)

	for _, id := range nonExistentIds {
		endpoint, err := s.Get(id)
		if err != nil {
			return err
		}
		err = s.Remove(endpoint, handlers...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) with(f func()) {
	s.m.Lock()
	defer s.m.Unlock()
	f()
}
