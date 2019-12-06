package watcher

import (
	"fmt"
	"testing"
	"time"

	"github.com/Demonware/balanced/pkg/util"

	"github.com/golang/mock/gomock"

	"github.com/Demonware/balanced/mocks"
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	coreinformer "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
)

type ControllerTestSuite struct {
	suite.Suite
	cs                clientset.Interface
	informers         informers.SharedInformerFactory
	serviceInformer   coreinformer.ServiceInformer
	endpointsInformer coreinformer.EndpointsInformer
}

func (suite *ControllerTestSuite) SetupTest() {
	suite.cs = testclient.NewSimpleClientset()
	suite.informers = informers.NewSharedInformerFactory(suite.cs, 0)
	suite.serviceInformer = suite.informers.Core().V1().Services()
	suite.endpointsInformer = suite.informers.Core().V1().Endpoints()

}

func (suite *ControllerTestSuite) TestInformersHasSyncedFuncs() {
	cA := NewController(
		suite.cs,
		suite.serviceInformer,
		suite.endpointsInformer,
		nil,
		nil,
		nil,
		nil,
	)
	podInformer := suite.informers.Core().V1().Pods()
	cB := NewController(
		suite.cs,
		suite.serviceInformer,
		suite.endpointsInformer,
		[]cache.SharedIndexInformer{podInformer.Informer()},
		nil,
		nil,
		nil,
	)

	cases := []struct {
		controller            *Controller
		expectedHasSyncedFunc []cache.InformerSynced
	}{
		{cA,
			[]cache.InformerSynced{
				suite.serviceInformer.Informer().HasSynced,
				suite.endpointsInformer.Informer().HasSynced,
			},
		},
		{cB,
			[]cache.InformerSynced{
				suite.serviceInformer.Informer().HasSynced,
				suite.endpointsInformer.Informer().HasSynced,
				podInformer.Informer().HasSynced,
			},
		},
	}
	for _, c := range cases {
		actualFns := c.controller.informerHasSyncedFuncs()
		assert.Equal(suite.T(), len(c.expectedHasSyncedFunc), len(actualFns))
		// TODO: check pointer equality to functions using reflect
	}

}

func (suite *ControllerTestSuite) TestIsValidServiceLoadBalancer() {
	kubeSvcA := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barA",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeClusterIP,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	kubeSvcB := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barB",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
		Status: api.ServiceStatus{
			LoadBalancer: api.LoadBalancerStatus{
				Ingress: []api.LoadBalancerIngress{
					api.LoadBalancerIngress{
						IP: "192.168.0.100",
					},
				},
			},
		},
	}

	cases := []struct {
		svc      *api.Service
		expected bool
	}{
		{kubeSvcA, false},
		{kubeSvcB, true},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, isValidServiceLoadBalancer(c.svc))
	}
}

func (suite *ControllerTestSuite) TestMatchingEndpoints() {
	kubeSvc := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
		Status: api.ServiceStatus{
			LoadBalancer: api.LoadBalancerStatus{
				Ingress: []api.LoadBalancerIngress{
					api.LoadBalancerIngress{
						IP: "192.168.0.100",
					},
				},
			},
		},
	}
	kubeEndpoints := &api.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Subsets: []api.EndpointSubset{},
	}

	stop := make(chan struct{})
	cs := testclient.NewSimpleClientset()
	inf := informers.NewSharedInformerFactory(cs, 0)
	serviceInformer := inf.Core().V1().Services()
	endpointsInformer := inf.Core().V1().Endpoints()

	_, err := cs.Core().Services("foo").Create(kubeSvc)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	c := NewController(
		cs,
		serviceInformer,
		endpointsInformer,
		nil,
		nil,
		nil,
		nil,
	)
	// Make sure informers are running.
	inf.Start(stop)
	endpoint, err := c.matchingEndpoints(kubeSvc)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), endpoint)

	_, err = cs.Core().Endpoints("foo").Create(kubeEndpoints)
	if err != nil {
		suite.T().Errorf("error injecting endpoints add: %v", err)
	}
	cache.WaitForCacheSync(
		stop,
		serviceInformer.Informer().HasSynced,
		endpointsInformer.Informer().HasSynced,
	)
	endpoints, err := c.matchingEndpoints(kubeSvc)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), endpoints, kubeEndpoints)
}

func (suite *ControllerTestSuite) TestPopulateServiceManager() {
	var onServiceChangeCalled = 0
	var onEndpointChangeCalled = 0

	var onServiceChangeCalledFn = func(prevSvc *v1alpha1.Service, svc *v1alpha1.Service) {
		suite.T().Logf("onServiceChangeFn Called: %+v", svc)
		assert.Nil(suite.T(), prevSvc, "should only be an add operation, no prevSvc should be specified")
		onServiceChangeCalled++
	}
	var onEndpointChangeCalledFn = func(prevE *v1alpha1.Endpoint, e *v1alpha1.Endpoint) {
		suite.T().Logf("onEndpointChangeFn Called: %+v", e)
		assert.Nil(suite.T(), prevE, "should only be an add operation, no prevE should be specified")
		onEndpointChangeCalled++
	}
	kubeSvcClusterIP := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "but",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeClusterIP,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	kubeSvc := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
					Name:       "http",
				},
				{
					Protocol:   "TCP",
					Port:       int32(443),
					TargetPort: intstr.FromInt(443),
					NodePort:   int32(34152),
					Name:       "https",
				},
			},
			ClusterIP: "100.64.0.111",
		},
		Status: api.ServiceStatus{
			LoadBalancer: api.LoadBalancerStatus{
				Ingress: []api.LoadBalancerIngress{
					api.LoadBalancerIngress{
						IP: "192.168.0.100",
					},
				},
			},
		},
	}
	nodeName := "test"
	kubeEndpoints := &api.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Subsets: []api.EndpointSubset{
			{
				Addresses: []api.EndpointAddress{
					{
						IP:       "100.96.100.123",
						NodeName: &nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-pod-1234",
							Namespace: "foo",
						},
					},
				},
				Ports: []api.EndpointPort{
					{
						Name:     "https",
						Port:     int32(443),
						Protocol: "TCP",
					},
				},
			},
		},
	}

	stop := make(chan struct{})
	cs := testclient.NewSimpleClientset()
	inf := informers.NewSharedInformerFactory(cs, 0)
	serviceInformer := inf.Core().V1().Services()
	endpointsInformer := inf.Core().V1().Endpoints()

	chs := &ControllerHandlers{
		InitServiceOnChangeHandlers:   []v1alpha1.ServiceOnChangeHandler{onServiceChangeCalledFn},
		InitEndpointsOnChangeHandlers: []v1alpha1.EndpointOnChangeHandler{onEndpointChangeCalledFn},
	}
	namedLogger := util.NewNamedLogger("testLogger", 1)
	sm := v1alpha1.NewMapServiceManager(namedLogger)
	c := NewController(
		cs,
		serviceInformer,
		endpointsInformer,
		nil,
		nil,
		sm,
		chs,
	)
	err := c.populateServiceManager()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(sm.List()))

	// Make sure informers are running.
	inf.Start(stop)
	_, err = cs.Core().Services("foo").Create(kubeSvc)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	_, err = cs.Core().Services("foo").Create(kubeSvcClusterIP)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	_, err = cs.Core().Endpoints("foo").Create(kubeEndpoints)
	if err != nil {
		suite.T().Errorf("error injecting endpoints add: %v", err)
	}
	cache.WaitForCacheSync(
		stop,
		serviceInformer.Informer().HasSynced,
		endpointsInformer.Informer().HasSynced,
	)

	err = c.populateServiceManager()
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, onServiceChangeCalled)
	assert.Equal(suite.T(), 1, onEndpointChangeCalled)
}

func (suite *ControllerTestSuite) TestHasInformedSynced() {
	stop := make(chan struct{})
	cs := testclient.NewSimpleClientset()
	inf := informers.NewSharedInformerFactory(cs, 0)
	serviceInformer := inf.Core().V1().Services()
	endpointsInformer := inf.Core().V1().Endpoints()

	c := NewController(
		cs,
		serviceInformer,
		endpointsInformer,
		nil,
		nil,
		nil,
		nil,
	)

	assert.Error(suite.T(), c.hasInformersSynced(c.informerHasSyncedFuncs()))

	inf.Start(stop)

	kubeSvc := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "but",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeClusterIP,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	_, err := cs.Core().Services("foo").Create(kubeSvc)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	for !serviceInformer.Informer().HasSynced() || !endpointsInformer.Informer().HasSynced() {
		time.Sleep(10 * time.Millisecond)
	}
	cache.WaitForCacheSync(
		stop,
		serviceInformer.Informer().HasSynced,
		endpointsInformer.Informer().HasSynced,
	)
	assert.NoError(suite.T(), c.hasInformersSynced(c.informerHasSyncedFuncs()))
}

func (suite *ControllerTestSuite) TestAddKeyToWorkQueue() {
	controller := NewController(
		suite.cs,
		suite.serviceInformer,
		suite.endpointsInformer,
		nil,
		nil,
		nil,
		nil,
	)

	cases := []struct {
		key         string
		expectedErr bool
	}{
		{"kube-system/kube-controller-manager", true},
		{"kube-system/kube-scheduler", true},
		{"default/kubernetes", true},
		{"foo/bar", false},
		{"baz/buzz", false},
	}
	for _, c := range cases {
		err := controller.addKeyToWorkQueue(c.key)
		assert.Equal(suite.T(), c.expectedErr, err != nil)
	}
}

func (suite *ControllerTestSuite) TestSyncServiceManagerNonexistentService() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockSm := mocks.NewMockServiceManager(mockCtrl)
	chs := &ControllerHandlers{}
	controller := NewController(
		suite.cs,
		suite.serviceInformer,
		suite.endpointsInformer,
		nil,
		nil,
		mockSm,
		chs,
	)
	testKey := "test/nonexistent-svc"
	existingSvcs := []*v1alpha1.Service{}
	mockSm.EXPECT().ResolveByKubeMetaNamespaceKey(testKey).Return(nil, fmt.Errorf("test error")).Times(1)
	mockSm.EXPECT().ResolveByKubeMetaNamespaceKey(testKey).Return(existingSvcs, nil).Times(1)
	mockSm.EXPECT().RemoveServicesRecursively(existingSvcs, chs.ServiceOnChangeHandlers, chs.EndpointsOnChangeHandlers).Return(nil).Times(1)

	assert.Error(suite.T(), controller.syncServiceManager(testKey))
	assert.NoError(suite.T(), controller.syncServiceManager(testKey))
}

func (suite *ControllerTestSuite) TestSyncServiceManager() {
	kubeSvcClusterIP := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "but",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeClusterIP,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	kubeSvc := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
					Name:       "http",
				},
				{
					Protocol:   "TCP",
					Port:       int32(443),
					TargetPort: intstr.FromInt(443),
					NodePort:   int32(34152),
					Name:       "https",
				},
			},
			ClusterIP: "100.64.0.111",
		},
		Status: api.ServiceStatus{
			LoadBalancer: api.LoadBalancerStatus{
				Ingress: []api.LoadBalancerIngress{
					api.LoadBalancerIngress{
						IP: "192.168.0.100",
					},
				},
			},
		},
	}
	nodeName := "test"
	kubeEndpoints := &api.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Subsets: []api.EndpointSubset{
			{
				Addresses: []api.EndpointAddress{
					{
						IP:       "100.96.100.123",
						NodeName: &nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-pod-1234",
							Namespace: "foo",
						},
					},
				},
				Ports: []api.EndpointPort{
					{
						Name:     "https",
						Port:     int32(443),
						Protocol: "TCP",
					},
				},
			},
		},
	}

	stop := make(chan struct{})
	cs := testclient.NewSimpleClientset()
	inf := informers.NewSharedInformerFactory(cs, 0)
	serviceInformer := inf.Core().V1().Services()
	endpointsInformer := inf.Core().V1().Endpoints()

	chs := &ControllerHandlers{}
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockSm := mocks.NewMockServiceManager(mockCtrl)
	c := NewController(
		cs,
		serviceInformer,
		endpointsInformer,
		nil,
		nil,
		mockSm,
		chs,
	)

	// Make sure informers are running.
	inf.Start(stop)
	_, err := cs.Core().Services("foo").Create(kubeSvc)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	_, err = cs.Core().Services("foo").Create(kubeSvcClusterIP)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	_, err = cs.Core().Endpoints("foo").Create(kubeEndpoints)
	if err != nil {
		suite.T().Errorf("error injecting endpoints add: %v", err)
	}

	cache.WaitForCacheSync(
		stop,
		serviceInformer.Informer().HasSynced,
		endpointsInformer.Informer().HasSynced,
	)

	testKey := "foo/bar"
	testKeyB := "foo/but"
	existingSvcs := []*v1alpha1.Service{}
	mockSm.EXPECT().ResolveByKubeMetaNamespaceKey(testKeyB).Return(existingSvcs, nil).Times(1)
	mockSm.EXPECT().ResolveByKubeMetaNamespaceKey(testKey).Return(existingSvcs, nil).Times(3)
	mockSm.EXPECT().UpdateServicesRecursively(gomock.Any(), gomock.Any(), chs.ServiceOnChangeHandlers, chs.EndpointsOnChangeHandlers).Return(fmt.Errorf("test")).Times(1)
	mockSm.EXPECT().UpdateServicesRecursively(gomock.Any(), gomock.Any(), chs.ServiceOnChangeHandlers, chs.EndpointsOnChangeHandlers).Return(nil).Times(2)
	mockSm.EXPECT().RemoveExistingServicesNoLongerCurrent(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("test")).Times(1)
	mockSm.EXPECT().RemoveExistingServicesNoLongerCurrent(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	assert.NoError(suite.T(), c.syncServiceManager(testKeyB))
	assert.Error(suite.T(), c.syncServiceManager(testKey))
	assert.Error(suite.T(), c.syncServiceManager(testKey))
	assert.NoError(suite.T(), c.syncServiceManager(testKey))
}

func (suite *ControllerTestSuite) TestExecutePostSyncHandlers() {
	var postSyncCalled = 0
	postSyncCalledFn := func() error {
		postSyncCalled++
		return nil
	}
	postSyncCalledErrorFn := func() error {
		return fmt.Errorf("test err")
	}
	chs := &ControllerHandlers{
		PostSyncHandlers: []PostSyncHandler{postSyncCalledFn, postSyncCalledFn},
	}
	c := NewController(
		suite.cs,
		suite.serviceInformer,
		suite.endpointsInformer,
		nil,
		nil,
		nil,
		chs,
	)
	assert.NoError(suite.T(), c.executePostSyncHandlers())
	assert.Equal(suite.T(), 2, postSyncCalled)

	chs.PostSyncHandlers = []PostSyncHandler{postSyncCalledErrorFn}
	assert.Error(suite.T(), c.executePostSyncHandlers())
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestControllerSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}
