package main

import (
	"net"
	"testing"

	"github.com/Demonware/balanced/pkg/util"

	"k8s.io/client-go/tools/record"

	"k8s.io/client-go/tools/cache"

	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	"k8s.io/client-go/informers"
	coreinformer "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

type VIPReconcilerTestSuite struct {
	suite.Suite
	cs              clientset.Interface
	informers       informers.SharedInformerFactory
	serviceInformer coreinformer.ServiceInformer
	defaultCidr     net.IPNet
	eventRecorder   record.EventRecorder
}

func (suite *VIPReconcilerTestSuite) SetupTest() {
	suite.cs = testclient.NewSimpleClientset()
	suite.informers = informers.NewSharedInformerFactory(suite.cs, 0)
	suite.serviceInformer = suite.informers.Core().V1().Services()
	eventBroadcaster := record.NewBroadcasterForTests(0)
	suite.eventRecorder = eventBroadcaster.NewRecorder(scheme.Scheme, api.EventSource{Component: "eventTest"})

	var ip = net.ParseIP("192.168.1.1")
	suite.defaultCidr = net.IPNet{
		IP:   ip,
		Mask: net.IPv4Mask(255, 255, 255, 0),
	}
}

func getStrRef(str string) *string {
	return &str
}

func (suite *VIPReconcilerTestSuite) TestAcquireFromSpecifiedIPs() {
	kubeSvcA := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barA",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
			Name:      "barA",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	svcA, _ := v1alpha1.NewService(kubeSvcA, kubeSvcA.Spec.Ports[0])
	svcB, _ := v1alpha1.NewService(kubeSvcB, kubeSvcB.Spec.Ports[0])
	specIPA := []string{"192.168.0.100"}
	specIPB := []string{
		"192.168.0.100",
		"192.168.0.101",
		"192.168.0.102",
	}
	cases := []struct {
		svc     *v1alpha1.Service
		reqIPs  []string
		freeIPs []string

		expectedIP    string
		expectedError bool
	}{
		{svcA, specIPA, []string{"192.168.0.100"}, "192.168.0.100", false},
		{svcA, specIPA, []string{}, "", true},
		{svcA, specIPA, nil, "", true},
		{svcB, specIPB, []string{"192.168.0.100"}, "192.168.0.100", false},
		{svcB, specIPB, []string{"192.168.0.101", "192.168.0.102"}, "192.168.0.101", false},
		{svcB, specIPB, []string{"192.168.0.102"}, "192.168.0.102", false},
		{svcB, specIPB, []string{"192.168.0.110"}, "", true},
		{svcB, specIPB, []string{}, "", true},
		{svcB, specIPB, nil, "", true},
	}
	for _, c := range cases {
		ip, err := acquireFromSpecifiedIPs(c.svc, c.reqIPs, c.freeIPs)
		if !c.expectedError {
			assert.Equal(suite.T(), c.expectedIP, ip, "assert IP address is equal to expected")
			assert.NoError(suite.T(), err, "assert no error has occured when acquiring IP")
		} else {
			assert.Error(suite.T(), err, "assert an error has occured when acquiring IP")
		}
	}
}

func (suite *VIPReconcilerTestSuite) TestIPPoolName() {
	cases := []struct {
		ipPoolName         *string
		expectedIPPoolName string
	}{
		{getStrRef("test"), "test"},
		{nil, ""},
	}
	for _, c := range cases {
		vipReconciler := NewVIPReconciler(suite.cs, c.ipPoolName, &suite.defaultCidr, suite.serviceInformer, nil, nil)
		assert.Equal(suite.T(), c.expectedIPPoolName, vipReconciler.ipPoolName())
	}
}

func (suite *VIPReconcilerTestSuite) TestIsSelectedPool() {
	// test VIP reconciler with non-specified IP Pool
	vipReconcilerNonSpecifiedPool := NewVIPReconciler(suite.cs, nil, &suite.defaultCidr, suite.serviceInformer, nil, nil)
	vipReconcilerTestPool := NewVIPReconciler(suite.cs, getStrRef("test"), &suite.defaultCidr, suite.serviceInformer, nil, nil)

	kubeSvcA := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barA",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
	kubeSvcB := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barB",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "test",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
	kubeSvcC := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barC",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
	kubeSvcD := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barD",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "192.168.1.1/24",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
	kubeSvcE := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "barE",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "192.168.2.1/24",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
	svcA, _ := v1alpha1.NewService(kubeSvcA, kubeSvcA.Spec.Ports[0])
	svcB, _ := v1alpha1.NewService(kubeSvcB, kubeSvcB.Spec.Ports[0])
	svcC, _ := v1alpha1.NewService(kubeSvcC, kubeSvcC.Spec.Ports[0])
	svcD, _ := v1alpha1.NewService(kubeSvcD, kubeSvcD.Spec.Ports[0])
	svcE, _ := v1alpha1.NewService(kubeSvcE, kubeSvcE.Spec.Ports[0])
	cases := []struct {
		vipR           *VIPReconciler
		svc            *v1alpha1.Service
		expectedResult bool
	}{
		{vipReconcilerNonSpecifiedPool, svcA, true},
		{vipReconcilerNonSpecifiedPool, svcB, false},
		{vipReconcilerNonSpecifiedPool, svcC, true},
		{vipReconcilerNonSpecifiedPool, svcD, true},
		{vipReconcilerNonSpecifiedPool, svcE, false},
		{vipReconcilerTestPool, svcA, false},
		{vipReconcilerTestPool, svcB, true},
		{vipReconcilerTestPool, svcC,
			false},
		{vipReconcilerTestPool, svcD, true},
		{vipReconcilerTestPool, svcE, false},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expectedResult, c.vipR.isSelectedPool(c.svc))
	}
}

func (suite *VIPReconcilerTestSuite) TestAllIPs() {
	vipReconciler := NewVIPReconciler(
		suite.cs,
		nil,
		&suite.defaultCidr,
		suite.serviceInformer,
		nil,
		nil,
	)

	allIps := vipReconciler.allIPs()
	assert.Equal(suite.T(), 254, len(allIps))
	assert.Equal(suite.T(), "192.168.1.1", allIps[0])
	assert.Equal(suite.T(), "192.168.1.254", allIps[len(allIps)-1])

	var ip = net.ParseIP("192.168.1.100")
	cidr := net.IPNet{
		IP:   ip,
		Mask: net.IPv4Mask(255, 255, 255, 255),
	}

	vipReconcilerWithOneIPAddress := NewVIPReconciler(
		suite.cs,
		nil,
		&cidr,
		suite.serviceInformer,
		nil,
		nil,
	)
	allIps = vipReconcilerWithOneIPAddress.allIPs()
	assert.Equal(suite.T(), 1, len(allIps))
	assert.Equal(suite.T(), "192.168.1.100", allIps[0])
}

func getServiceManager() *v1alpha1.MapServiceManager {
	testLogger := util.NewNamedLogger("testLogger", 1)
	sm := v1alpha1.NewMapServiceManager(testLogger)
	kubeSvcA := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "192.168.2.1/24",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
			Name:      "baz",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "192.168.2.1/24",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
				api.ServicePort{
					Protocol:   "TCP",
					Port:       int32(443),
					TargetPort: intstr.FromInt(443),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
		Status: api.ServiceStatus{
			LoadBalancer: api.LoadBalancerStatus{
				Ingress: []api.LoadBalancerIngress{
					api.LoadBalancerIngress{
						IP: "192.168.1.100",
					},
				},
			},
		},
	}
	kubeSvcC := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bum",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "192.168.2.1/24",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
						IP: "192.168.1.2",
					},
				},
			},
		},
	}
	// ignore errors as we're not testing MapServiceManager or Service here..
	svcA, _ := v1alpha1.NewService(kubeSvcA, kubeSvcA.Spec.Ports[0])
	svcB, _ := v1alpha1.NewService(kubeSvcB, kubeSvcB.Spec.Ports[0])
	svcC, _ := v1alpha1.NewService(kubeSvcB, kubeSvcB.Spec.Ports[1])
	svcD, _ := v1alpha1.NewService(kubeSvcC, kubeSvcC.Spec.Ports[0])
	_ = sm.Add(svcA)
	_ = sm.Add(svcB)
	_ = sm.Add(svcC)
	_ = sm.Add(svcD)

	return sm
}

func (suite *VIPReconcilerTestSuite) TestExistingIPs() {
	testLogger := util.NewNamedLogger("testLogger", 1)
	vipReconciler := NewVIPReconciler(
		suite.cs,
		nil,
		&suite.defaultCidr,
		suite.serviceInformer,
		nil,
		v1alpha1.NewMapServiceManager(testLogger),
	)
	assert.Empty(suite.T(), vipReconciler.existingIPs())

	sm := getServiceManager()
	vipReconciler = NewVIPReconciler(
		suite.cs,
		nil,
		&suite.defaultCidr,
		suite.serviceInformer,
		nil,
		sm,
	)
	existingIPs := vipReconciler.existingIPs()
	expectedIPs := []string{"192.168.1.2", "192.168.1.100", "192.168.1.100"}

	for _, expectedIP := range expectedIPs {
		assert.Contains(suite.T(), existingIPs, expectedIP)
	}
	assert.Equal(suite.T(), len(expectedIPs), len(existingIPs))
}

func (suite *VIPReconcilerTestSuite) TestFreeIPs() {
	sm := getServiceManager()
	vipReconciler := NewVIPReconciler(
		suite.cs,
		nil,
		&suite.defaultCidr,
		suite.serviceInformer,
		nil,
		sm,
	)
	freeIPs := vipReconciler.FreeIPs()
	assert.Equal(suite.T(), 252, len(freeIPs))
	assert.NotContains(suite.T(), freeIPs, []string{"192.168.1.100", "192.168.1.2"})
}

func (suite *VIPReconcilerTestSuite) TestAssignVIPIfValidService() {
	stop := make(chan struct{})
	// We will create an informer that writes added pods to a channel.
	svcs := make(chan *api.Service, 2)
	svcInformer := suite.serviceInformer.Informer()
	svcInformer.AddEventHandler(&cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(prevObj interface{}, obj interface{}) {
			svc := obj.(*api.Service)
			suite.T().Logf("service updated: %s/%s", svc.Namespace, svc.Name)
			svcs <- svc
		},
	})
	// Make sure informers are running.
	suite.informers.Start(stop)

	kubeSvcA := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bum",
			Namespace: "foo",
			Annotations: map[string]string{
				ipPoolAnnotation: "192.168.2.1/24", // test not selected pool
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
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
						IP: "192.168.1.2",
					},
				},
			},
		},
	}
	kubeSvcC := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
			Namespace: "foo",
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	kubeSvcD := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "badummD",
			Namespace: "foo",
			Annotations: map[string]string{
				"balanced.demonware.net/ip-request": "192.168.1.100, 192.168.1.101, 192.168.1.102",
			},
		},
		Spec: api.ServiceSpec{
			Type: api.ServiceTypeLoadBalancer,
			Ports: []api.ServicePort{
				api.ServicePort{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	_, err := suite.cs.Core().Services("foo").Create(kubeSvcC)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	_, err = suite.cs.Core().Services("foo").Create(kubeSvcD)
	if err != nil {
		suite.T().Errorf("error injecting svc add: %v", err)
	}
	svcA, _ := v1alpha1.NewService(kubeSvcA, kubeSvcA.Spec.Ports[0])
	svcB, _ := v1alpha1.NewService(kubeSvcB, kubeSvcB.Spec.Ports[0])
	svcC, _ := v1alpha1.NewService(kubeSvcC, kubeSvcC.Spec.Ports[0])
	svcD, _ := v1alpha1.NewService(kubeSvcD, kubeSvcD.Spec.Ports[0])
	sm := getServiceManager()
	sm.Add(svcB)
	vipReconcilerWithNoIps := NewVIPReconciler(
		suite.cs,
		nil,
		&net.IPNet{
			IP:   net.ParseIP("192.168.1.2"),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		suite.serviceInformer,
		suite.eventRecorder,
		sm,
	)
	vipReconcilerWithNoIps.AssignVIPIfValidService(nil, svcC)
	vipReconcilerWithNoIps.AssignVIPIfValidService(nil, svcD)
	sm = getServiceManager()
	sm.Add(svcD)
	vipReconciler := NewVIPReconciler(
		suite.cs,
		nil,
		&suite.defaultCidr,
		suite.serviceInformer,
		suite.eventRecorder,
		sm,
	)
	vipReconciler.AssignVIPIfValidService(svcA, nil)
	vipReconciler.AssignVIPIfValidService(nil, svcA)
	vipReconciler.AssignVIPIfValidService(nil, svcB)
	vipReconciler.AssignVIPIfValidService(nil, svcC)
	vipReconciler.AssignVIPIfValidService(nil, svcD)

	// Wait and check result.
	cache.WaitForCacheSync(
		stop,
		svcInformer.HasSynced,
	)

	for i := 0; i < 2; i++ {
		select {
		case svc := <-svcs:
			suite.T().Logf("Got svc from channel: %s/%s", svc.Namespace, svc.Name)
			assert.NotEmpty(suite.T(), svc.Status.LoadBalancer.Ingress)
			if len(svc.Status.LoadBalancer.Ingress) != 0 {
				assert.NotEmpty(suite.T(), svc.Status.LoadBalancer.Ingress[0].IP)
			} else {
				suite.T().Error("Service has no Ingress objects")
			}
		default:
			suite.T().Error("Informer did not get the updated pod")
		}
	}
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestVIPReconcilerSuite(t *testing.T) {
	suite.Run(t, new(VIPReconcilerTestSuite))
}
