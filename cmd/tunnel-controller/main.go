package main

import (
	"flag"
	"fmt"

	"github.com/Demonware/balanced/pkg/pidresolver"

	"github.com/golang/glog"
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	"github.com/Demonware/balanced/pkg/health"
	"github.com/Demonware/balanced/pkg/metrics"
	"github.com/Demonware/balanced/pkg/util"
	"github.com/Demonware/balanced/pkg/watcher"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	controllerName = "balanced-tunnel-controller"
)

var (
	loadKernelModules bool
	resolverType      string
)

func tunnelConfigurator(resolverType pidresolver.ResolverType, eventRecorder record.EventRecorder, pidResolver pidresolver.PIDResolver) TunnelConfigurator {
	switch resolverType {
	case pidresolver.DummyResolver:
		glog.Infof("TunnelConfigurator: %s", "dummy")
		return &DummyTunnelConfigurator{}
	default:
		glog.Infof("TunnelConfigurator: %s", "network namespace")
		return &NamespaceTunnelConfigurator{
			eventRecorder: eventRecorder,
			pidResolver:   pidResolver,
		}
	}
}

func realMain(conf *rest.Config, val interface{}, mp *metrics.MetricsPublisher, hm *health.HealthManager) ([]util.RunFunc, error) {
	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, err
	}
	if loadKernelModules {
		err = util.LoadKernelModule("ipip")
		if err != nil {
			return nil, err
		}
	}
	var cfg = val.(*config)
	pidR, err := pidResolver(cfg.Resolver, pidresolver.ResolverType(resolverType))
	if err != nil {
		return nil, err
	}

	var (
		coreInformer     = coreinformers.NewSharedInformerFactory(clientset, util.ResyncPeriod)
		filteredInformer = coreinformers.NewSharedInformerFactoryWithOptions(clientset, util.ResyncPeriod, coreinformers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fmt.Sprintf("spec.nodeName=%s", util.Hostname)
		}))
		serviceInformer  = coreInformer.Core().V1().Services()
		endpointInformer = coreInformer.Core().V1().Endpoints()
		_                = NewTunnelMetricsExporter()
		podInformer      = filteredInformer.Core().V1().Pods()
		eventRecorder    = util.NewEventRecorder(
			clientset,
			fmt.Sprintf("%s, %s", controllerName, util.Hostname))

		smLogger         = util.NewNamedLogger(controllerName, 3)
		serviceManager   = v1alpha1.NewMapServiceManager(smLogger)
		tunnelReconciler = NewTunnelReconciler(
			util.Hostname, podInformer, serviceManager,
			tunnelConfigurator(pidresolver.ResolverType(resolverType), eventRecorder, pidR),
			NewThreadsafeHostPodCache())
		watcherHealth      = health.NewGenericHealthCheck("watcherSync", util.ResyncPeriod+util.WatcherSyncDur)
		controllerHandlers = &watcher.ControllerHandlers{
			InitEndpointsOnChangeHandlers: []v1alpha1.EndpointOnChangeHandler{tunnelReconciler.OnChangeEndpointHandler},
			EndpointsOnChangeHandlers:     []v1alpha1.EndpointOnChangeHandler{tunnelReconciler.OnChangeEndpointHandler},
			PostSyncHandlers:              []watcher.PostSyncHandler{tunnelReconciler.Sync},
		}
		controller = watcher.NewController(
			clientset,
			serviceInformer,
			endpointInformer,
			[]cache.SharedIndexInformer{podInformer.Informer()},
			watcherHealth,
			serviceManager,
			controllerHandlers,
		)
	)
	hm.RegisterCheck(watcherHealth)

	return []util.RunFunc{coreInformer.Start, filteredInformer.Start, controller.Run(cfg.Controller.Concurrency)}, nil
}

func main() {
	flag.StringVar(&resolverType, "resolver", "dynamic", "The resolver used to resolve container PID (supported: dynamic, docker, containerd, dummy [for testing purposes])")
	flag.BoolVar(&loadKernelModules, "load_kernel_modules", true, "Load required kernel modules on start-up of controller")
	util.Main(realMain, &config{
		Controller: watcher.ConfigController{
			Concurrency: 2,
		},
		Resolver: pidresolver.ConfigResolver{
			Containerd: &pidresolver.ConfigContainerResolver{
				Endpoint: "unix:///run/containerd/containerd.sock", // default containerd socket
			},
		},
	})
}
