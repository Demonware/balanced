package v1alpha1

import (
	"fmt"
	"net"
	"reflect"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ServiceTestSuite struct {
	suite.Suite
	kubeSvcA      *api.Service
	svcA          *Service
	kubeSvcB      *api.Service
	svcB          *Service
	kubeEndpoints *api.Endpoints
	endpoints     []*Endpoint
	httpEndpoint  *Endpoint
	httpsEndpoint *Endpoint
}

func (suite *ServiceTestSuite) SetupTest() {
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
	suite.svcA, err = NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	suite.kubeSvcB = &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
			Annotations: map[string]string{
				"test": "test",
			},
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
					Protocol:   "UDP",
					Port:       int32(53),
					TargetPort: intstr.FromInt(53),
					NodePort:   int32(34152),
					Name:       "dns",
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	suite.svcB, err = NewService(suite.kubeSvcB, suite.kubeSvcB.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	nodeName := "test"
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
						NodeName: &nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-pod-1234",
							Namespace: "foo",
						},
					},
					{
						IP:       "100.96.100.124",
						NodeName: &nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-pod-12345",
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
	suite.endpoints, err = ConvertKubeAPIV1EndpointsToEndpoints(suite.kubeEndpoints)
	if err != nil {
		suite.T().Errorf("cannot convert endpoints for testing")
	}
	for _, endpoint := range suite.endpoints {
		if endpoint.PortName() == "http" {
			suite.httpEndpoint = endpoint
		} else if endpoint.PortName() == "https" {
			suite.httpsEndpoint = endpoint
		}
	}
}

func (suite *ServiceTestSuite) TestGetLoadBalancerIP() {
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

	expectedKubeSvcIP := net.ParseIP("192.168.0.100")
	cases := []struct {
		svc        *api.Service
		expectedIP *net.IP
	}{
		{kubeSvcClusterIP, nil},
		{suite.kubeSvcA, &expectedKubeSvcIP},
		{suite.kubeSvcB, nil},
	}
	for _, c := range cases {
		assert.True(
			suite.T(),
			reflect.DeepEqual(c.expectedIP, getLoadBalancerIP(c.svc)),
			fmt.Sprintf("getLoadBalancerIP not returning what is expected: %s != %s", getLoadBalancerIP(c.svc), c.expectedIP),
		)
	}
}

func (suite *ServiceTestSuite) TestConvertKubeAPIV1ServiceToServices() {
	// this function assumes that kubeService is a Service Type: Loadbalancer, which requires ports to be defined
	serviceOptSuccessFn := func(*Service) error {
		return nil
	}
	serviceOptErrFn := func(*Service) error {
		return fmt.Errorf("err")
	}
	cases := []struct {
		kubeSvc              *api.Service
		opts                 []ServiceOption
		expectedNoOfServices int
		expectedErr          bool
	}{
		{suite.kubeSvcA, nil, 2, false},
		{suite.kubeSvcA, []ServiceOption{serviceOptErrFn}, 0, true},
		{suite.kubeSvcA, []ServiceOption{serviceOptSuccessFn, serviceOptErrFn}, 0, true},
		{suite.kubeSvcA, []ServiceOption{serviceOptSuccessFn}, 2, false},
	}
	for _, c := range cases {
		actualSvc, err := ConvertKubeAPIV1ServiceToServices(c.kubeSvc, c.opts...)
		assert.Equal(suite.T(), c.expectedNoOfServices, len(actualSvc))
		assert.Equal(suite.T(), c.expectedErr, err != nil)
	}
}

func (suite *ServiceTestSuite) TestTargetPort() {
	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.Nil(suite.T(), svc.TargetPort())

	_, err = svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}
	expectedTargetPort := uint16(80)
	assert.True(suite.T(), reflect.DeepEqual(svc.TargetPort(), &expectedTargetPort))
}

func (suite *ServiceTestSuite) TestID() {
	assert.Equal(suite.T(), suite.svcA.ID(), "foo/bar/TCP/80", suite.svcA.ID())
}

func (suite *ServiceTestSuite) TestKubeAPIV1ServiceName() {
	assert.Equal(suite.T(), "foo/bar", suite.svcA.KubeAPIV1ServiceName())
}

func (suite *ServiceTestSuite) TestKubeAPIV1Service() {
	assert.Equal(suite.T(), suite.kubeSvcA, suite.svcA.KubeAPIV1Service())
}

func (suite *ServiceTestSuite) TestAddress() {
	expectedIPAddress := net.ParseIP("192.168.0.100")
	assert.NotNil(suite.T(), suite.svcA.Address())
	assert.Equal(suite.T(), suite.svcA.Address(), &expectedIPAddress)
	svcB, err := NewService(suite.kubeSvcB, suite.kubeSvcB.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.Nil(suite.T(), svcB.Address())
}

func (suite *ServiceTestSuite) TestPort() {
	assert.Equal(suite.T(), uint16(80), suite.svcA.Port())
}

func (suite *ServiceTestSuite) TestPortName() {
	assert.Equal(suite.T(), "http", suite.svcA.PortName())
}

func (suite *ServiceTestSuite) TestProtocol() {
	assert.Equal(suite.T(), api.Protocol("TCP"), suite.svcA.Protocol())
	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[1]) // protocol is lower-cased in PortSpec
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.Equal(suite.T(), api.Protocol("TCP"), svc.Protocol())
}

func (suite *ServiceTestSuite) TestProtoNumber() {
	assert.Equal(suite.T(), uint16(syscall.IPPROTO_TCP), suite.svcA.ProtoNumber())
	svc, err := NewService(suite.kubeSvcB, suite.kubeSvcB.Spec.Ports[1])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.Equal(suite.T(), uint16(syscall.IPPROTO_UDP), svc.ProtoNumber())
}

func (suite *ServiceTestSuite) TestIsEqualAddress() {
	svcA, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	kubeSvcNoIPAddress := &api.Service{
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
	svcNoAddress, err := NewService(kubeSvcNoIPAddress, kubeSvcNoIPAddress.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}

	cases := []struct {
		baseSvc     *Service
		comparedSvc *Service
		expected    bool
	}{
		{suite.svcA, svcA, true},
		{suite.svcA, svcNoAddress, false},
		{svcNoAddress, suite.svcA, false},
		{svcNoAddress, svcNoAddress, true},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, c.baseSvc.isEqualAddress(c.comparedSvc))
	}
}

func (suite *ServiceTestSuite) TestEquals() {
	kubeSvcAWithDifferentAnnotations := &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
			Annotations: map[string]string{
				"test:": "test",
			},
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
	svcADiffAnnotation, err := NewService(
		kubeSvcAWithDifferentAnnotations,
		kubeSvcAWithDifferentAnnotations.Spec.Ports[0],
	)
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}

	cases := []struct {
		baseSvc     *Service
		comparedSvc *Service
		expected    bool
	}{
		{suite.svcA, suite.svcA, true},
		{suite.svcA, suite.svcB, false},
		{suite.svcB, suite.svcA, false},
		{suite.svcA, svcADiffAnnotation, false},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, c.baseSvc.Equals(c.comparedSvc))
	}
}

func (suite *ServiceTestSuite) TestAnnotations() {
	assert.Empty(suite.T(), suite.svcA.Annotations())
	assert.True(suite.T(), reflect.DeepEqual(suite.svcB.Annotations(), map[string]string{"test": "test"}))
}

func (suite *ServiceTestSuite) TestIsChildEndpoint() {
	assert.False(suite.T(), suite.svcA.IsChildEndpoint(suite.httpsEndpoint))
	assert.True(suite.T(), suite.svcA.IsChildEndpoint(suite.httpEndpoint))
}

func (suite *ServiceTestSuite) TestExists() {
	assert.False(suite.T(), suite.svcA.Exists(suite.httpEndpoint))

	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	_, err = svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}

	assert.True(suite.T(), svc.Exists(suite.httpEndpoint))
	assert.False(suite.T(), svc.Exists(suite.httpsEndpoint))
}

func (suite *ServiceTestSuite) TestGet() {
	_, err := suite.svcA.Get(suite.httpEndpoint.ID())
	assert.Error(suite.T(), err)

	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	_, err = svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}
	actual, err := svc.Get(suite.httpEndpoint.ID())
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), suite.httpEndpoint, actual)
}

func (suite *ServiceTestSuite) TestList() {
	assert.Empty(suite.T(), suite.svcA.List())

	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	_, err = svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}
	assert.Len(suite.T(), svc.List(), 2)
}

func (suite *ServiceTestSuite) TestAdd() {
	var onEndpointHandlerCalled int
	onEndpointHandlerCalledFn := func(prevE *Endpoint, e *Endpoint) {
		onEndpointHandlerCalled++
	}
	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.NoError(suite.T(), svc.Add(suite.httpEndpoint))
	assert.Error(suite.T(), svc.Add(suite.httpEndpoint))
	assert.Error(suite.T(), svc.Add(suite.httpEndpoint, onEndpointHandlerCalledFn, onEndpointHandlerCalledFn))
	assert.Equal(suite.T(), 0, onEndpointHandlerCalled)
	assert.NoError(suite.T(), svc.Add(suite.httpsEndpoint, onEndpointHandlerCalledFn, onEndpointHandlerCalledFn))
	assert.Equal(suite.T(), 2, onEndpointHandlerCalled)
}

func (suite *ServiceTestSuite) TestUpdate() {
	assert.Error(suite.T(), suite.svcA.Update(suite.httpEndpoint))

	var onEndpointHandlerCalled int
	assertNoChangeFn := func(prevE *Endpoint, e *Endpoint) {
		assert.Equal(suite.T(), prevE, e)
		onEndpointHandlerCalled++
	}
	assertChangeFn := func(prevE *Endpoint, e *Endpoint) {
		assert.NotEqual(suite.T(), prevE, e)
		onEndpointHandlerCalled++
	}
	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.NoError(suite.T(), svc.Add(suite.httpEndpoint))
	assert.NoError(suite.T(), svc.Update(suite.httpEndpoint, assertNoChangeFn))
	assert.Equal(suite.T(), 1, onEndpointHandlerCalled)

	// TODO: fix brittle slice index based test
	address := suite.kubeEndpoints.Subsets[0].Addresses[1]
	port := suite.kubeEndpoints.Subsets[0].Ports[1]
	notReadyEndpoint, err := NewEndpoint(
		suite.kubeEndpoints,
		&address,
		&port,
		Readiness(false),
	)
	if err != nil {
		suite.T().Errorf("cannot create new endpoints for testing")
	}
	assert.NoError(suite.T(), svc.Update(notReadyEndpoint, assertChangeFn))
	assert.Equal(suite.T(), 2, onEndpointHandlerCalled)
}

func (suite *ServiceTestSuite) TestRemove() {
	assert.Error(suite.T(), suite.svcA.Remove(suite.httpEndpoint))
	var onEndpointHandlerCalled int
	onEndpointHandlerCalledFn := func(prevE *Endpoint, e *Endpoint) {
		onEndpointHandlerCalled++
	}

	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	assert.NoError(suite.T(), svc.Add(suite.httpEndpoint))
	assert.NoError(suite.T(), svc.Add(suite.httpsEndpoint))
	assert.NoError(suite.T(), svc.Remove(suite.httpEndpoint, onEndpointHandlerCalledFn, onEndpointHandlerCalledFn))
	assert.Equal(suite.T(), 2, onEndpointHandlerCalled)
}

func (suite *ServiceTestSuite) TestUpsertEndpoints() {

	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	result, err := svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}
	assert.Equal(suite.T(), 2, len(result.Added))
	assert.Equal(suite.T(), 0, len(result.Updated))
	assert.Equal(suite.T(), 2, len(result.Ignored))

	// upsert again
	result, err = svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}
	assert.Equal(suite.T(), 0, len(result.Added))
	assert.Equal(suite.T(), 2, len(result.Updated))
	assert.Equal(suite.T(), 2, len(result.Ignored))
}

func (suite *ServiceTestSuite) TestDeleteEndpointsNotSupplied() {
	assertDeleteEndpointHandlerCalledFn := func(prevE *Endpoint, e *Endpoint) {
		assert.NotNil(suite.T(), prevE)
		assert.Nil(suite.T(), e)
	}

	assert.NoError(suite.T(), suite.svcA.deleteEndpointsNotSupplied(suite.endpoints))
	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	_, err = svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}

	// TODO: fix brittle slice index based test
	nodeName := "test"
	address := &api.EndpointAddress{
		IP:       "100.96.100.125",
		NodeName: &nodeName,
		TargetRef: &api.ObjectReference{
			Kind:      "Pod",
			Name:      "i-am-a-pod-123456",
			Namespace: "foo",
		},
	}
	port := suite.kubeEndpoints.Subsets[0].Ports[1]
	endpoint, err := NewEndpoint(
		suite.kubeEndpoints,
		address,
		&port,
		Readiness(false),
	)
	err = svc.Add(endpoint)
	if err != nil {
		suite.T().Errorf("cannot add new endpoints for testing")
	}
	assert.NoError(suite.T(), svc.deleteEndpointsNotSupplied(suite.endpoints, assertDeleteEndpointHandlerCalledFn))
}

func (suite *ServiceTestSuite) TestUpdateEndpoints() {
	var updated int
	var removed int
	assertChangeCountFn := func(prevE *Endpoint, e *Endpoint) {
		if prevE != nil && e == nil {
			removed++
		}
		if prevE != nil && e != nil {
			updated++
		}
	}

	assert.NoError(suite.T(), suite.svcA.deleteEndpointsNotSupplied(suite.endpoints))
	svc, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	_, err = svc.UpsertEndpoints(suite.endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints to svc")
	}

	// TODO: fix brittle slice index based test
	nodeName := "test"
	address := &api.EndpointAddress{
		IP:       "100.96.100.125",
		NodeName: &nodeName,
		TargetRef: &api.ObjectReference{
			Kind:      "Pod",
			Name:      "i-am-a-pod-123456",
			Namespace: "foo",
		},
	}
	port := suite.kubeEndpoints.Subsets[0].Ports[1]
	endpoint, err := NewEndpoint(
		suite.kubeEndpoints,
		address,
		&port,
		Readiness(false),
	)
	err = svc.Add(endpoint)
	if err != nil {
		suite.T().Errorf("cannot add new endpoints for testing")
	}
	assert.Equal(suite.T(), 3, len(svc.List()))
	result, err := svc.UpdateEndpoints(suite.endpoints, assertChangeCountFn)
	if err != nil {
		suite.T().Errorf("cannot update endpoints to svc")
	}
	assert.Equal(suite.T(), 0, len(result.Added))
	assert.Equal(suite.T(), 2, len(result.Updated))
	assert.Equal(suite.T(), 2, len(result.Ignored))
	assert.Equal(suite.T(), 2, len(svc.List()))
	assert.Equal(suite.T(), 2, updated)
	assert.Equal(suite.T(), 1, removed)
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestServiceSuite(t *testing.T) {
	suite.Run(t, new(ServiceTestSuite))
}
