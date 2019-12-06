package v1alpha1

import (
	"fmt"
	"net"

	api "k8s.io/api/core/v1"
)

type Endpoint struct {
	kubeEndpoints       *api.Endpoints
	kubeEndpointAddress *api.EndpointAddress
	kubeEndpointPort    *api.EndpointPort

	svc   *Service
	ready bool
}

type EndpointOption func(*Endpoint) error

func Readiness(ready bool) EndpointOption {
	return func(e *Endpoint) error {
		e.ready = ready
		return nil
	}
}

// ConvertKubeAPIV1EndpointsToEndpoints translates kubernetes endpoints objects to balanced endpoints
func ConvertKubeAPIV1EndpointsToEndpoints(kubeEndpoints *api.Endpoints) (endpoints []*Endpoint, err error) {
	// this looks like an O(n^3) loop, but .Subsets are always just a list of one element
	for _, subset := range kubeEndpoints.Subsets {
		for _, port := range subset.Ports {
			ePort := port // copy loop variable before taking its reference
			newEndpointsFn := func(addrs []api.EndpointAddress, readiness bool) error {
				for _, a := range addrs {
					eAddr := a // copy loop variable before taking its reference
					e, err := NewEndpoint(kubeEndpoints, &eAddr, &ePort, Readiness(readiness))
					if err != nil {
						return err
					}
					endpoints = append(endpoints, e)
				}
				return nil
			}
			if err = newEndpointsFn(subset.Addresses, true); err != nil {
				return
			}
			if err = newEndpointsFn(subset.NotReadyAddresses, false); err != nil {
				return
			}
		}
	}
	return
}

func NewEndpoint(kubeEndpoints *api.Endpoints, kubeEndpointAddress *api.EndpointAddress, kubeEndpointPort *api.EndpointPort, opts ...EndpointOption) (*Endpoint, error) {
	e := &Endpoint{
		kubeEndpoints:       kubeEndpoints,
		kubeEndpointAddress: kubeEndpointAddress,
		kubeEndpointPort:    kubeEndpointPort,
	}
	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}
	return e, nil
}

func (e *Endpoint) ID() string {
	return fmt.Sprintf(
		"%s/%d",
		e.Address().String(),
		e.Port())
}

func (e *Endpoint) SetService(svc *Service) {
	e.svc = svc
	return
}

func (e *Endpoint) Service() *Service {
	return e.svc
}

func (e *Endpoint) KubeAPIV1EndpointsName() string {
	return e.kubeEndpoints.GetNamespace() + "/" + e.kubeEndpoints.GetName()
}

func (e *Endpoint) KubeAPIV1Endpoints() *api.Endpoints {
	return e.kubeEndpoints
}

func (e *Endpoint) Address() net.IP {
	return net.ParseIP(e.kubeEndpointAddress.IP)
}

func (e *Endpoint) PortName() string {
	return e.kubeEndpointPort.Name
}

func (e *Endpoint) Port() uint16 {
	return uint16(e.kubeEndpointPort.Port)
}

func (e *Endpoint) TargetRef() *api.ObjectReference {
	return e.kubeEndpointAddress.TargetRef
}

func (e *Endpoint) TargetRefString() string {
	var targetRefStr = ""
	if e.IsTargetPod() {
		targetRefStr = fmt.Sprintf("TargetType: Pod [%s/%s]", e.TargetRef().Namespace, e.TargetRef().Name)
	}
	return targetRefStr
}

func (e *Endpoint) IsTargetPod() bool {
	if e.kubeEndpointAddress.TargetRef != nil {
		return e.kubeEndpointAddress.TargetRef.Kind == "Pod"
	}
	return false
}

func (e *Endpoint) NodeName() *string {
	return e.kubeEndpointAddress.NodeName
}

func (e *Endpoint) Ready() bool {
	return e.ready
}

func (e *Endpoint) isEqualTarget(endpoint *Endpoint) bool {
	if e.TargetRef() == endpoint.TargetRef() {
		return true
	} else if e.TargetRef() == nil && endpoint.TargetRef() != nil {
		return false
	} else if e.TargetRef() != nil && endpoint.TargetRef() == nil {
		return false
	}
	return e.TargetRef() != nil && endpoint.TargetRef() != nil && e.TargetRef().UID == endpoint.TargetRef().UID
}

func (e *Endpoint) Equals(endpoint *Endpoint) bool {
	equalName := e.ID() == endpoint.ID()
	equalReadiness := e.Ready() == endpoint.Ready()
	return equalName && e.isEqualTarget(endpoint) && equalReadiness
}
