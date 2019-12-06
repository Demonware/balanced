package main

import (
	"flag"
	"fmt"
	"net"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"

	"github.com/Demonware/balanced/pkg/health"
	"github.com/Demonware/balanced/pkg/metrics"
	"github.com/Demonware/balanced/pkg/util"
	"github.com/Demonware/balanced/pkg/watcher"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	controllerName = "balanced-vip-allocation-controller"
)

var (
	cidrStr    string
	ipPoolName *string
)

func realMain(conf *rest.Config, _ interface{}, mp *metrics.MetricsPublisher, hm *health.HealthManager) ([]util.RunFunc, error) {
	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, err
	}
	if cidrStr == "" {
		return nil, fmt.Errorf("CIDR is required to be specified. Use -cidr")
	}
	_, ipnet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return nil, fmt.Errorf("User supplied CIDR is invalid (%s): %s", cidrStr, err.Error())
	}

	var (
		coreInformer       = coreinformers.NewSharedInformerFactory(clientset, util.ResyncPeriod)
		serviceInformer    = coreInformer.Core().V1().Services()
		endpointInformer   = coreInformer.Core().V1().Endpoints()
		_                  = NewVIPMetricsExporter()
		eventRecorder      = util.NewEventRecorder(clientset, fmt.Sprintf("%s, ip-pool: %s", controllerName, *ipPoolName))
		watcherHealth      = health.NewGenericHealthCheck("watcherSync", util.ResyncPeriod+util.WatcherSyncDur)
		smLogger           = util.NewNamedLogger(controllerName, 3)
		serviceManager     = v1alpha1.NewMapServiceManager(smLogger)
		vipReconciler      = NewVIPReconciler(clientset, ipPoolName, ipnet, serviceInformer, eventRecorder, serviceManager)
		controllerHandlers = &watcher.ControllerHandlers{
			ServiceOnChangeHandlers: []v1alpha1.ServiceOnChangeHandler{vipReconciler.AssignVIPIfValidService},
			PostSyncHandlers:        []watcher.PostSyncHandler{vipReconciler.RefreshMetrics},
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
	return []util.RunFunc{coreInformer.Start, controller.Run(1)}, nil
}

func main() {
	flag.StringVar(&cidrStr, "cidr", "", "VIP CIDR assigned to controller for allocation (eg. 10.49.248.0/24)")
	ipPoolName = flag.String("ip_pool", "", "IP Pool name (eg. default)")

	util.Main(realMain, &config{
		Controller: watcher.ConfigController{},
	})
}
