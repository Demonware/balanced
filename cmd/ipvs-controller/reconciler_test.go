package main

import (
	"net"
	"reflect"
	"syscall"
	"testing"

	utilipvs "github.com/Demonware/balanced/pkg/util/ipvs"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	api "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/docker/libnetwork/ipvs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/vishvananda/netlink/nl"
)

type IpvsReconcilerTestSuite struct {
	suite.Suite
	kubeSvcA *api.Service
	svcA     *v1alpha1.Service

	nodeName      string
	kubeEndpoints *api.Endpoints
	endpointA     *v1alpha1.Endpoint
	endpointB     *v1alpha1.Endpoint
}

func (suite *IpvsReconcilerTestSuite) SetupTest() {
	suite.kubeSvcA = &api.Service{
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
					Protocol:   "tcp",
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
	var err error
	suite.svcA, err = v1alpha1.NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}

	suite.nodeName = "test"
	suite.kubeEndpoints = &api.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Subsets: []api.EndpointSubset{
			{
				Addresses: []api.EndpointAddress{
					{
						IP:       "100.96.100.123",
						NodeName: &suite.nodeName,
						TargetRef: &api.ObjectReference{
							UID:       "test",
							Kind:      "Pod",
							Name:      "i-am-a-pod-1234",
							Namespace: "foo",
						},
					},
					{
						IP:       "100.96.100.124",
						NodeName: &suite.nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-pod-12345",
							Namespace: "foo",
						},
					},
				},
				NotReadyAddresses: []api.EndpointAddress{
					{
						IP:       "100.96.100.125",
						NodeName: &suite.nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-dead-pod-1234",
							Namespace: "foo",
						},
					},
					{
						IP:       "100.96.100.126",
						NodeName: &suite.nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-dead-pod-12345",
							Namespace: "foo",
						},
					},
					{
						IP:       "100.96.100.127",
						NodeName: &suite.nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-dead-pod-123456",
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
					{
						Name:     "http",
						Port:     int32(80),
						Protocol: "TCP",
					},
				},
			},
		},
	}
	address := suite.kubeEndpoints.Subsets[0].Addresses[0]
	port := suite.kubeEndpoints.Subsets[0].Ports[1]
	suite.endpointA, err = v1alpha1.NewEndpoint(
		suite.kubeEndpoints,
		&address,
		&port,
		v1alpha1.Readiness(false),
	)
	if err != nil {
		suite.T().Errorf("cannot create endpoint A")
	}
	suite.endpointB, err = v1alpha1.NewEndpoint(
		suite.kubeEndpoints,
		&address,
		&port,
		v1alpha1.Readiness(true),
	)
	if err != nil {
		suite.T().Errorf("cannot create endpoint A")
	}
}

func (suite *IpvsReconcilerTestSuite) TestIsIPInCIDRs() {
	_, cidr, err := net.ParseCIDR("192.168.0.0/24")
	if err != nil {
		suite.T().Error("cannot parse cidr")
	}
	_, cidr2, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		suite.T().Error("cannot parse cidr")
	}
	defaultIpvsScheduler := "default"
	ir := NewIpvsReconciler([]*net.IPNet{cidr}, nil, nil, nil, defaultIpvsScheduler)
	ir2 := NewIpvsReconciler([]*net.IPNet{cidr, cidr2}, nil, nil, nil, defaultIpvsScheduler)
	cases := []struct {
		reconciler *IpvsReconciler
		ip         net.IP
		expected   bool
	}{
		{ir, net.ParseIP("192.168.0.100"), true},
		{ir, net.ParseIP("10.0.100.1"), false},
		{ir2, net.ParseIP("192.168.1.2"), true},
		{ir2, net.ParseIP("192.168.0.2"), true},
		{ir2, net.ParseIP("192.168.3.2"), false},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, c.reconciler.isIPinCIDRs(c.ip))
	}
}

func (suite *IpvsReconcilerTestSuite) TestIsValidIPAddress() {
	_, cidr, err := net.ParseCIDR("192.168.0.0/24")
	if err != nil {
		suite.T().Error("cannot parse cidr")
	}
	defaultIpvsScheduler := "default"
	ir := NewIpvsReconciler([]*net.IPNet{cidr}, nil, nil, nil, defaultIpvsScheduler)

	kubeSvcNoIP := &api.Service{
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
					Protocol:   "tcp",
					Port:       int32(443),
					TargetPort: intstr.FromInt(443),
					NodePort:   int32(34152),
					Name:       "https",
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	svcNoIP, err := v1alpha1.NewService(kubeSvcNoIP, kubeSvcNoIP.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}

	_, cidr2, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		suite.T().Error("cannot parse cidr")
	}
	ir2 := NewIpvsReconciler([]*net.IPNet{cidr2}, nil, nil, nil, defaultIpvsScheduler)

	cases := []struct {
		reconciler *IpvsReconciler
		svc        *v1alpha1.Service
		expected   bool
	}{
		{ir, nil, false},
		{ir, suite.svcA, true},
		{ir, svcNoIP, false},
		{ir2, suite.svcA, false},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, c.reconciler.isValidIPAddress(c.svc))
	}
}

func (suite *IpvsReconcilerTestSuite) TestIsValidEndpoint() {
	assert.False(suite.T(), isValidEndpoint(suite.endpointA))
	suite.endpointA.SetService(suite.svcA)
	assert.True(suite.T(), isValidEndpoint(suite.endpointA))

	svcB, err := v1alpha1.NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[1])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	suite.endpointA.SetService(svcB)
	assert.False(suite.T(), isValidEndpoint(suite.endpointA))

}

func (suite *IpvsReconcilerTestSuite) TestConvertEndpointToIpvsDestination() {
	id1 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.123"),
		Port:            80,
		Weight:          0,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	id2 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.123"),
		Port:            80,
		Weight:          1,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	cases := []struct {
		endpoint     *v1alpha1.Endpoint
		expectedDest *ipvs.Destination
	}{
		{suite.endpointA, id1},
		{suite.endpointB, id2},
	}
	for _, c := range cases {
		assert.True(suite.T(), reflect.DeepEqual(c.expectedDest, ConvertEndpointToIpvsDestination(c.endpoint)))
	}
}

func (suite *IpvsReconcilerTestSuite) TestGetIpvsScheduler() {
	_, cidr, err := net.ParseCIDR("192.168.0.0/24")
	if err != nil {
		suite.T().Error("cannot parse cidr")
	}
	defaultIpvsScheduler := "default"
	ir := NewIpvsReconciler([]*net.IPNet{cidr}, nil, nil, nil, defaultIpvsScheduler)
	assert.Equal(suite.T(), defaultIpvsScheduler, ir.getIpvsScheduler(suite.svcA))

	for _, ipvsScheduler := range utilipvs.SupportedIpvsSchedulers {
		kubeSvcAWithAnnotations := suite.kubeSvcA.DeepCopy()
		kubeSvcAWithAnnotations.ObjectMeta.Annotations = map[string]string{
			ipvsSchedulerAnnotation: ipvsScheduler,
		}
		svcAWithAnnotations, err := v1alpha1.NewService(kubeSvcAWithAnnotations, kubeSvcAWithAnnotations.Spec.Ports[0])
		if err != nil {
			suite.T().Errorf("cannot create new service for testing")
		}
		assert.Equal(suite.T(), ipvsScheduler, ir.getIpvsScheduler(svcAWithAnnotations))
	}

	kubeSvcAWithAnnotations := suite.kubeSvcA.DeepCopy()
	kubeSvcAWithAnnotations.ObjectMeta.Annotations = map[string]string{
		ipvsSchedulerAnnotation: "doesnotexist",
	}
	svcAWithAnnotations, err := v1alpha1.NewService(kubeSvcAWithAnnotations, kubeSvcAWithAnnotations.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.Equal(suite.T(), defaultIpvsScheduler, ir.getIpvsScheduler(svcAWithAnnotations))
}

func (suite *IpvsReconcilerTestSuite) TestConvertServiceToIpvsService() {
	defaultIpvsScheduler := "default"
	expectedSvc := &ipvs.Service{
		Address:       net.ParseIP("192.168.0.100"),
		AddressFamily: syscall.AF_INET,
		Protocol:      uint16(syscall.IPPROTO_TCP),
		Port:          80,
		SchedName:     defaultIpvsScheduler,
	}
	_, cidr, err := net.ParseCIDR("192.168.0.0/24")
	if err != nil {
		suite.T().Error("cannot parse cidr")
	}
	ir := NewIpvsReconciler([]*net.IPNet{cidr}, nil, nil, nil, defaultIpvsScheduler)

	assert.True(suite.T(), reflect.DeepEqual(expectedSvc, ir.ConvertServiceToIpvsService(suite.svcA)))
}

func (suite *IpvsReconcilerTestSuite) TestHasIpvsServiceChanged() {
	defaultIpvsScheduler := "default"
	iSvc := &ipvs.Service{
		Address:       net.ParseIP("192.168.0.100"),
		AddressFamily: syscall.AF_INET,
		Protocol:      uint16(syscall.IPPROTO_TCP),
		Port:          80,
		SchedName:     defaultIpvsScheduler,
	}
	_, cidr, err := net.ParseCIDR("192.168.0.0/24")
	if err != nil {
		suite.T().Error("cannot parse cidr")
	}
	ir := NewIpvsReconciler([]*net.IPNet{cidr}, nil, nil, nil, defaultIpvsScheduler)

	assert.False(suite.T(), ir.hasIpvsServiceChanged(iSvc, suite.svcA))
	kubeSvcAWithAnnotations := suite.kubeSvcA.DeepCopy()
	kubeSvcAWithAnnotations.ObjectMeta.Annotations = map[string]string{
		ipvsSchedulerAnnotation: utilipvs.SupportedIpvsSchedulers[0],
	}
	svcAWithAnnotations, err := v1alpha1.NewService(kubeSvcAWithAnnotations, kubeSvcAWithAnnotations.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.True(suite.T(), ir.hasIpvsServiceChanged(iSvc, svcAWithAnnotations))
}

func (suite *IpvsReconcilerTestSuite) TestEndpointInIpvsDestinations() {
	id1 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.124"),
		Port:            80,
		Weight:          1,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	id2 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.124"),
		Port:            8080,
		Weight:          1,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	id3 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.123"),
		Port:            8080,
		Weight:          1,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	id4 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.123"),
		Port:            80,
		Weight:          1,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	cases := []struct {
		iDests       []*ipvs.Destination
		endpoint     *v1alpha1.Endpoint
		expectedDest *ipvs.Destination
	}{
		{[]*ipvs.Destination{}, suite.endpointA, nil},
		{[]*ipvs.Destination{id4}, suite.endpointA, id4},
		{[]*ipvs.Destination{id1, id2, id3, id4}, suite.endpointA, id4},
	}
	for _, c := range cases {
		assert.True(suite.T(), reflect.DeepEqual(c.expectedDest, endpointInIpvsDestinations(c.iDests, c.endpoint)))
	}
}

func (suite *IpvsReconcilerTestSuite) TestHasIpvsDestinationChanged() {
	id1 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.123"),
		Port:            80,
		Weight:          1,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	id2 := &ipvs.Destination{
		AddressFamily:   nl.FAMILY_V4,
		Address:         net.ParseIP("100.96.100.123"),
		Port:            80,
		Weight:          0,
		ConnectionFlags: ipvs.ConnectionFlagTunnel,
	}
	cases := []struct {
		iDest    *ipvs.Destination
		endpoint *v1alpha1.Endpoint
		expected bool
	}{
		{id1, suite.endpointA, true},
		{id2, suite.endpointA, false},
		{id1, suite.endpointB, false},
		{id2, suite.endpointB, true},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, hasIpvsDestinationChanged(c.iDest, c.endpoint))
	}
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestIpvsReconcilerSuite(t *testing.T) {
	suite.Run(t, new(IpvsReconcilerTestSuite))
}
