package v1alpha1

// ServiceManager manages balanced Service objects
type ServiceManager interface {
	Exists(svc *Service) bool
	// ResolveByKubeMetaNamespaceKey : finds all Services with the same Kubernetes Service Object Key
	ResolveByKubeMetaNamespaceKey(key string) (svcs []*Service, err error)
	// RemoveExistingServicesNoLongerCurrent removes existing services that are managed by the MapServiceManager that are no longer in existence in the real world
	RemoveExistingServicesNoLongerCurrent(currentServices []*Service, existingServices []*Service, handlers ...ServiceOnChangeHandler) (err error)
	Get(svcID string) (svc *Service, err error)
	List() (list []*Service)
	ListServicesWithAddress() (list []*Service)
	Add(svc *Service, handlers ...ServiceOnChangeHandler) (err error)
	Update(svc *Service, handlers ...ServiceOnChangeHandler) (updated bool, prevSvc *Service, err error)
	Remove(svc *Service, handlers ...ServiceOnChangeHandler) (err error)
	RemoveByID(svcID string, handlers ...ServiceOnChangeHandler) (err error)
	// RemoveServicesRecursively remove Endpoints of a Service first before removal
	RemoveServicesRecursively(svcs []*Service, svcOnChangeHandler []ServiceOnChangeHandler, eOnChangeHandler []EndpointOnChangeHandler) error
	// UpdateServicesRecursively will add Service to SM if not already exist and its Endpoints,
	// otherwise, it will update the existing Service in the SM along with its Endpoints
	UpdateServicesRecursively(svcs []*Service, endpoints []*Endpoint, svcOnChangeHandler []ServiceOnChangeHandler, eOnChangeHandler []EndpointOnChangeHandler) error
}
