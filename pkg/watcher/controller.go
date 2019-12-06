package watcher

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/davecgh/go-spew/spew"
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"

	"github.com/golang/glog"
	"github.com/Demonware/balanced/pkg/health"
	api "k8s.io/api/core/v1"
	coreinformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var (
	// These endpoints trigger way too often - every time a lock gets renewed
	//   Filter these out to prevent excessive syncs
	endpointsToSkip = []string{
		"kube-system/kube-controller-manager",
		"kube-system/kube-scheduler",
		"default/kubernetes",
	}
)

type Controller struct {
	queue               workqueue.RateLimitingInterface
	clientset           kubernetes.Interface
	serviceInformer     cache.SharedIndexInformer
	endpointInformer    cache.SharedIndexInformer
	additionalInformers []cache.SharedIndexInformer
	healthCheck         *health.GenericHealthCheck
	serviceManager      v1alpha1.ServiceManager
	chs                 *ControllerHandlers
}

func (c *Controller) informerHasSyncedFuncs() []cache.InformerSynced {
	var hasSyncedFuncs []cache.InformerSynced

	hasSyncedFuncs = []cache.InformerSynced{
		c.serviceInformer.HasSynced,
		c.endpointInformer.HasSynced,
	}
	for _, informer := range c.additionalInformers {
		hasSyncedFuncs = append(hasSyncedFuncs, informer.HasSynced)
	}

	return hasSyncedFuncs
}

func (c *Controller) Run(workers uint) func(<-chan struct{}) {
	return func(stopc <-chan struct{}) {
		defer c.queue.ShutDown()
		var hasSynced []cache.InformerSynced

		glog.Infof("Starting BalanceD Watcher Controller (concurrency: %d)", workers)

		hasSynced = c.informerHasSyncedFuncs()
		if !cache.WaitForCacheSync(stopc, hasSynced...) {
			panic("Error with cache sync")
		}
		err := c.populateServiceManager()
		if err != nil {
			glog.Infof("Error initializing ServiceManager: %s", err.Error())
			panic("Error populating service manager")
		}
		for i := uint(0); i < workers; i++ {
			go wait.Until(c.runWorker, time.Second, stopc)
		}
		go c.setupSignalHandler()
		<-stopc
		glog.Infof("Stopping BalanceD Watcher controller")
	}
}

func (c *Controller) setupSignalHandler() {
	sigc := make(chan os.Signal)
	signal.Notify(sigc, syscall.SIGUSR1)
	go func() {
		for {
			s := <-sigc
			switch s {
			case syscall.SIGUSR1:
				glog.Infof(c.spewServiceManager())
			}
		}
	}()
}

// spewServiceManager: used for debugging state of the Service Manager
func (c *Controller) spewServiceManager() string {
	return fmt.Sprintf("MapServiceManager: %+v", spew.Sdump(c.serviceManager))
}

func (c *Controller) matchingEndpoints(svc *api.Service) (*api.Endpoints, error) {
	svcName, err := cache.MetaNamespaceKeyFunc(svc)
	if err != nil {
		return nil, err
	}
	endObj, exists, err := c.endpointInformer.GetIndexer().GetByKey(svcName)
	if err != nil {
		return nil, err
	}
	if exists {
		return endObj.(*api.Endpoints), nil
	}
	return nil, nil
}

func isValidServiceLoadBalancer(svc *api.Service) bool {
	if svc.Spec.Type != api.ServiceTypeLoadBalancer {
		return false
	}
	return true
}

func (c *Controller) populateServiceManager() error {
	glog.V(2).Infof("Populating MapServiceManager with Services...")
	for _, obj := range c.serviceInformer.GetStore().List() {
		var (
			kubeSvc       *api.Service
			kubeEndpoints *api.Endpoints
			svcs          []*v1alpha1.Service
		)
		kubeSvc = obj.(*api.Service)
		if !isValidServiceLoadBalancer(kubeSvc) {
			continue
		}
		svcs, err := v1alpha1.ConvertKubeAPIV1ServiceToServices(kubeSvc)
		if err != nil {
			return err
		}
		kubeEndpoints, err = c.matchingEndpoints(kubeSvc)
		if err != nil {
			return err
		}
		endpoints, err := v1alpha1.ConvertKubeAPIV1EndpointsToEndpoints(kubeEndpoints)
		if err != nil {
			return err
		}
		for _, svc := range svcs {
			err := c.serviceManager.Add(svc, c.chs.InitServiceOnChangeHandlers...)
			if err != nil {
				return err
			}
			// parent Endpoints to Services
			_, err = svc.UpsertEndpoints(endpoints, c.chs.InitEndpointsOnChangeHandlers...)
			if err != nil {
				return err
			}
		}
	}
	glog.V(2).Infof("Populated MapServiceManager")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	if err := c.hasInformersSynced(c.informerHasSyncedFuncs()); err != nil {
		c.queue.AddRateLimited(key)
	}
	if err := c.syncServiceManager(key.(string)); err != nil {
		glog.Infof("Error during sync: %s", err)
		c.queue.AddRateLimited(key)
	} else {
		if err := c.executePostSyncHandlers(); err != nil {
			glog.Infof("Error during post sync: %s", err)
			c.queue.AddRateLimited(key)
		}
		c.executeHealthCheck()
		c.queue.Forget(key)
	}

	return true
}

func (c *Controller) hasInformersSynced(hasSyncedFuncs []cache.InformerSynced) error {
	for _, hasSyncedFunc := range hasSyncedFuncs {
		if !hasSyncedFunc() {
			return fmt.Errorf("not all informers have been synced")
		}
	}
	return nil
}

func (c *Controller) executePostSyncHandlers() error {
	for _, postSyncHandler := range c.chs.PostSyncHandlers {
		err := postSyncHandler()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) executeHealthCheck() {
	if c.healthCheck != nil {
		c.healthCheck.HeartBeat()
	}
}

func (c *Controller) syncServiceManager(key string) error {
	start := time.Now()
	defer func() {
		endTime := time.Since(start)
		nsEndTime := float64(endTime)
		SyncTimeMetric.Observe(nsEndTime)
		LastSyncTime.Set(nsEndTime)
	}()
	defer c.queue.Done(key)

	kubeSvcObj, kubeSvcExists, err := c.serviceInformer.GetIndexer().GetByKey(key)
	if err != nil {
		return err
	}
	kubeEndpointsObj, kubeEndpointsExists, err := c.endpointInformer.GetIndexer().GetByKey(key)
	if err != nil {
		return err
	}
	existingSvcs, err := c.serviceManager.ResolveByKubeMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if !kubeSvcExists {
		if kubeEndpointsExists {
			glog.V(3).Infof("Endpoints exists but cannot lookup Service. Aborting sync for safety: %s", key)
			return nil
		}
		// Kubernetes Service was removed from the APIServer
		return c.serviceManager.RemoveServicesRecursively(existingSvcs, c.chs.ServiceOnChangeHandlers, c.chs.EndpointsOnChangeHandlers)
	}
	kubeSvc := kubeSvcObj.(*api.Service)
	if !isValidServiceLoadBalancer(kubeSvc) {
		glog.V(4).Infof("not a valid LoadBalancer: %s", key)
		return nil
	}
	svcs, err := v1alpha1.ConvertKubeAPIV1ServiceToServices(kubeSvc)
	if err != nil {
		return err
	}
	var endpoints []*v1alpha1.Endpoint
	if kubeEndpointsObj != nil {
		kubeEndpoints := kubeEndpointsObj.(*api.Endpoints)
		endpoints, err = v1alpha1.ConvertKubeAPIV1EndpointsToEndpoints(kubeEndpoints)
		if err != nil {
			return err
		}
	}
	err = c.serviceManager.UpdateServicesRecursively(svcs, endpoints, c.chs.ServiceOnChangeHandlers, c.chs.EndpointsOnChangeHandlers)
	if err != nil {
		return err
	}
	err = c.serviceManager.RemoveExistingServicesNoLongerCurrent(svcs, existingSvcs, c.chs.ServiceOnChangeHandlers...)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) addKeyToWorkQueue(key string) error {
	for _, endpoint := range endpointsToSkip {
		if endpoint == key {
			return fmt.Errorf("key in endpointsToSkip, will not queue: %s", key)
		}
	}
	c.queue.Add(key)
	return nil
}

func NewController(
	kubeClient kubernetes.Interface, s coreinformer.ServiceInformer, e coreinformer.EndpointsInformer,
	additionalInformers []cache.SharedIndexInformer, healthCheck *health.GenericHealthCheck,
	serviceManager v1alpha1.ServiceManager,
	chs *ControllerHandlers) *Controller {

	c := &Controller{
		queue:               workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		clientset:           kubeClient,
		serviceInformer:     s.Informer(),
		endpointInformer:    e.Informer(),
		additionalInformers: additionalInformers,
		healthCheck:         healthCheck,
		serviceManager:      serviceManager,
		chs:                 chs,
	}

	eventHandler := &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				_ = c.addKeyToWorkQueue(key)
			}
		},
		UpdateFunc: func(obj interface{}, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				_ = c.addKeyToWorkQueue(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				_ = c.addKeyToWorkQueue(key)
			}
		},
	}
	c.serviceInformer.AddEventHandler(eventHandler)
	c.endpointInformer.AddEventHandler(eventHandler)
	return c
}
