package main

import (
	"flag"
	"fmt"
	"net"
	"strings"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"

	"github.com/Demonware/balanced/pkg/health"
	"github.com/Demonware/balanced/pkg/metrics"
	"github.com/Demonware/balanced/pkg/util"
	utilipvs "github.com/Demonware/balanced/pkg/util/ipvs"
	"github.com/Demonware/balanced/pkg/watcher"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/coreos/go-iptables/iptables"
)

const (
	controllerName = "balanced-ipvs-controller"
)

var (
	loadKernelModules    bool
	kubeProxyCompat      bool
	setIpvsKernelParams  bool
	cidrStr              string
	defaultIpvsScheduler string
)

func parseCidrs(cidrsStr string) (ipNets []*net.IPNet, err error) {
	cidrs := strings.Split(cidrsStr, ",")
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		ipNets = append(ipNets, ipNet)
	}
	return
}

func realMain(conf *rest.Config, val interface{}, mp *metrics.MetricsPublisher, hm *health.HealthManager) ([]util.RunFunc, error) {
	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, err
	}
	if cidrStr == "" {
		return nil, fmt.Errorf("CIDR is required to be specified. Use -cidr")
	}

	ipNets, err := parseCidrs(cidrStr)
	if err != nil {
		return nil, fmt.Errorf("User supplied CIDRs are invalid (%s): %s", cidrStr, err.Error())
	}
	if loadKernelModules {
		err = util.LoadKernelModule("ip_vs")
		if err != nil {
			return nil, err
		}
		utilipvs.LoadIpvsSchedulerKernelModulesIfExist()
	}
	if kubeProxyCompat {
		iptablesHandle, err := iptables.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create iptables handle: %s", err)
		}

		err = utilipvs.EnsureIptables(iptablesHandle, ipNets)
		if err != nil {
			return nil, fmt.Errorf("failed to setup iptables nat chain: %s", err)
		}
	}
	if setIpvsKernelParams {
		// enable ipvs connection tracking
		err = utilipvs.EnsureSysctlSettings()
		if err != nil {
			return nil, fmt.Errorf("failed to set ipvs sysctl's due to: %s", err)
		}
	}

	if !isValidDefaultScheduler() {
		return nil, fmt.Errorf("default scheduler not valid, choose from list: %+v", utilipvs.SupportedIpvsSchedulers)
	}

	var (
		cfg              = val.(*config)
		coreInformer     = coreinformers.NewSharedInformerFactory(clientset, util.ResyncPeriod)
		serviceInformer  = coreInformer.Core().V1().Services()
		endpointInformer = coreInformer.Core().V1().Endpoints()
		eventRecorder    = util.NewEventRecorder(clientset, fmt.Sprintf("%s, %s", controllerName, util.Hostname))
		smLogger         = util.NewNamedLogger(controllerName, 3)
		serviceManager   = v1alpha1.NewMapServiceManager(smLogger)
		watcherHealth    = health.NewGenericHealthCheck("watcherSync", util.ResyncPeriod+util.WatcherSyncDur)
		ipvsReconciler   = NewIpvsReconciler(ipNets, serviceInformer.Informer(), eventRecorder, serviceManager, defaultIpvsScheduler)
	)
	var (
		controllerHandlers = &watcher.ControllerHandlers{
			InitServiceOnChangeHandlers:   []v1alpha1.ServiceOnChangeHandler{ipvsReconciler.OnChangeServiceHandler},
			InitEndpointsOnChangeHandlers: []v1alpha1.EndpointOnChangeHandler{ipvsReconciler.OnChangeEndpointHandler},

			ServiceOnChangeHandlers:   []v1alpha1.ServiceOnChangeHandler{ipvsReconciler.OnChangeServiceHandler},
			EndpointsOnChangeHandlers: []v1alpha1.EndpointOnChangeHandler{ipvsReconciler.OnChangeEndpointHandler},
			PostSyncHandlers:          []watcher.PostSyncHandler{ipvsReconciler.CleanupOrphanedIpvs},
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

	metricsExporter := NewIpvsMetricsExporter(serviceManager)
	mp.Register(metricsExporter)
	hm.RegisterCheck(watcherHealth)

	return []util.RunFunc{coreInformer.Start, controller.Run(cfg.Controller.Concurrency)}, nil
}

func isValidDefaultScheduler() bool {
	validDefaultIpvsScheduler := false
	for _, ipvsSched := range utilipvs.SupportedIpvsSchedulers {
		if defaultIpvsScheduler == ipvsSched {
			validDefaultIpvsScheduler = true
			break
		}
	}
	return validDefaultIpvsScheduler
}

func main() {
	flag.StringVar(&cidrStr, "cidr", "", "VIP CIDR(s) that this controller will care about. Use commas as delimiter. (eg. 10.49.248.0/24 or 10.49.248.0/24,10.50.20.0/24)")
	flag.StringVar(&defaultIpvsScheduler, "default_ipvs_scheduler", utilipvs.HighestRandomWeightScheduler, "Default IPVS Scheduler for IPVS Service")
	flag.BoolVar(&loadKernelModules, "load_kernel_modules", true, "Load required kernel modules on start-up of controller")
	flag.BoolVar(&setIpvsKernelParams, "set_ipvs_kernel_params", true, "Set IPVS related kernel parameters using sysctl")
	flag.BoolVar(&kubeProxyCompat, "kube_proxy_compat", true, "Set to true if using this controller in the same network namespace as kube-proxy")

	util.Main(realMain, &config{
		Controller: watcher.ConfigController{
			Concurrency: 2,
		},
	})
}
