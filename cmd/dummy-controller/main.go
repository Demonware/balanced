package main

import (
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	"github.com/Demonware/balanced/pkg/health"
	"github.com/Demonware/balanced/pkg/metrics"
	"github.com/Demonware/balanced/pkg/watcher"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/Demonware/balanced/pkg/util"
	coreinformers "k8s.io/client-go/informers"
)

const (
	controllerName = "balanced-dummy-controller"
)

func realMain(conf *rest.Config, val interface{}, mp *metrics.MetricsPublisher, hm *health.HealthManager) ([]util.RunFunc, error) {
	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, err
	}
	var (
		cfg                = val.(*config)
		coreInformer       = coreinformers.NewSharedInformerFactory(clientset, util.ResyncPeriod)
		serviceInformer    = coreInformer.Core().V1().Services()
		endpointInformer   = coreInformer.Core().V1().Endpoints()
		_                  = NewDummyMetricsExporter()
		eventRecorder      = util.NewEventRecorder(clientset, controllerName)
		watcherHealth      = health.NewGenericHealthCheck("watcherSync", util.ResyncPeriod+util.WatcherSyncDur)
		smLogger           = util.NewNamedLogger(controllerName, 3)
		serviceManager     = v1alpha1.NewMapServiceManager(smLogger)
		dummyReconciler    = NewDummyReconciler(serviceInformer, eventRecorder, serviceManager)
		controllerHandlers = &watcher.ControllerHandlers{
			ServiceOnChangeHandlers:   []v1alpha1.ServiceOnChangeHandler{dummyReconciler.OnChangeServiceHandler},
			EndpointsOnChangeHandlers: []v1alpha1.EndpointOnChangeHandler{dummyReconciler.OnChangeEndpointHandler},
			PostSyncHandlers:          []watcher.PostSyncHandler{dummyReconciler.PostSyncHandler},
		}
		controller = watcher.NewController(
			clientset,
			serviceInformer,
			endpointInformer,
			nil,
			watcherHealth,
			serviceManager,
			controllerHandlers,
		)
	)
	hm.RegisterCheck(watcherHealth)
	return []util.RunFunc{coreInformer.Start, controller.Run(cfg.Controller.Concurrency)}, nil
}

func main() {
	util.Main(realMain, &config{
		Controller: watcher.ConfigController{
			Concurrency: 2,
		},
	})
}
