package watcher

import "github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"

type PostSyncHandler func() error

type ControllerHandlers struct {
	// handlers that would be called during controller start up; during service manager population
	// WARNING: do not rely on serviceManager.List() or .Get() operations during service manager population
	// as it will not contain the full set of Kubernetes Services and Endpoints
	InitServiceOnChangeHandlers   []v1alpha1.ServiceOnChangeHandler
	InitEndpointsOnChangeHandlers []v1alpha1.EndpointOnChangeHandler

	// handlers that would be called during actual Kubernetes service/endpoints changes
	// it is safe to rely on serviceManager.List() or .Get() operations at this point
	ServiceOnChangeHandlers   []v1alpha1.ServiceOnChangeHandler
	EndpointsOnChangeHandlers []v1alpha1.EndpointOnChangeHandler

	// handlers to be done post sync (eg. garbage collection)
	PostSyncHandlers []PostSyncHandler
}
