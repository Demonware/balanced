package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EndpointTestSuite struct {
	suite.Suite
	nodeName             string
	kubeEndpoints        *api.Endpoints
	kubeEndpointsNonPods *api.Endpoints
	endpointA            *Endpoint
	endpointNonPod       *Endpoint
}

func (suite *EndpointTestSuite) SetupTest() {
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
	suite.kubeEndpointsNonPods = &api.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Subsets: []api.EndpointSubset{
			{
				Addresses: []api.EndpointAddress{
					{
						IP: "100.96.101.123",
					},
					{
						IP: "100.96.101.124",
					},
				},
				NotReadyAddresses: []api.EndpointAddress{
					{
						IP: "100.96.101.125",
					},
					{
						IP: "100.96.101.126",
					},
					{
						IP: "100.96.101.127",
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
	var err error
	suite.endpointA, err = NewEndpoint(
		suite.kubeEndpoints,
		&address,
		&port,
		Readiness(false),
	)
	if err != nil {
		suite.T().Errorf("cannot create endpoint A")
	}
	addressB := suite.kubeEndpointsNonPods.Subsets[0].Addresses[0]
	portB := suite.kubeEndpointsNonPods.Subsets[0].Ports[1]

	suite.endpointNonPod, err = NewEndpoint(
		suite.kubeEndpoints,
		&addressB,
		&portB,
		Readiness(true),
	)
	if err != nil {
		suite.T().Errorf("cannot create endpointNonPod")
	}
}

func (suite *EndpointTestSuite) TestConvertKubeAPIV1EndpointsToEndpoints() {
	endpoints, err := ConvertKubeAPIV1EndpointsToEndpoints(suite.kubeEndpoints)
	assert.NoError(suite.T(), err)
	var (
		ready    int
		notReady int
	)
	for _, e := range endpoints {
		if e.Ready() {
			ready++
		} else {
			notReady++
		}
	}
	assert.Equal(suite.T(), 4, ready)
	assert.Equal(suite.T(), 6, notReady)
}

func (suite *EndpointTestSuite) TestID() {
	assert.Equal(suite.T(), "100.96.100.123/80", suite.endpointA.ID())
}

func (suite *EndpointTestSuite) TestService() {
	kubeSvcA := &api.Service{
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
	svcA, err := NewService(kubeSvcA, kubeSvcA.Spec.Ports[0])
	assert.NoError(suite.T(), err)

	assert.Nil(suite.T(), suite.endpointA.Service())
	suite.endpointA.SetService(svcA)
	assert.Equal(suite.T(), svcA, suite.endpointA.Service())
}

func (suite *EndpointTestSuite) TestKubeAPIV1EndpointsName() {
	assert.Equal(suite.T(), "foo/bar", suite.endpointA.KubeAPIV1EndpointsName())
}

func (suite *EndpointTestSuite) TestKubeAPIV1Endpoints() {
	assert.Equal(suite.T(), suite.kubeEndpoints, suite.endpointA.KubeAPIV1Endpoints())
}

func (suite *EndpointTestSuite) TestAddress() {
	assert.Equal(suite.T(), "100.96.100.123", suite.endpointA.Address().String())
}

func (suite *EndpointTestSuite) TestPortName() {
	assert.Equal(suite.T(), "http", suite.endpointA.PortName())
}

func (suite *EndpointTestSuite) TestPort() {
	assert.Equal(suite.T(), uint16(80), suite.endpointA.Port())
}

func (suite *EndpointTestSuite) TestTargetRef() {
	assert.Equal(suite.T(), suite.kubeEndpoints.Subsets[0].Addresses[0].TargetRef, suite.endpointA.TargetRef())
}

func (suite *EndpointTestSuite) TestTargetRefString() {
	assert.Equal(suite.T(), "TargetType: Pod [foo/i-am-a-pod-1234]", suite.endpointA.TargetRefString())
	assert.Equal(suite.T(), "", suite.endpointNonPod.TargetRefString())
}

func (suite *EndpointTestSuite) TestIsTargetPod() {
	assert.True(suite.T(), suite.endpointA.IsTargetPod())
	assert.False(suite.T(), suite.endpointNonPod.IsTargetPod())
}

func (suite *EndpointTestSuite) TestNodeName() {
	assert.Equal(suite.T(), &suite.nodeName, suite.endpointA.NodeName())
	assert.Nil(suite.T(), suite.endpointNonPod.NodeName())
}

func (suite *EndpointTestSuite) TestIsEqualTarget() {
	address := suite.kubeEndpoints.Subsets[0].Addresses[0].DeepCopy() // deep copy to get different targetref pointers
	port := suite.kubeEndpoints.Subsets[0].Ports[1]
	endpointA, err := NewEndpoint(
		suite.kubeEndpoints,
		address,
		&port,
		Readiness(false),
	)
	assert.NoError(suite.T(), err)

	addressB := suite.kubeEndpoints.Subsets[0].Addresses[1].DeepCopy() // deep copy to get different targetref pointers
	portB := suite.kubeEndpoints.Subsets[0].Ports[0]
	endpointB, err := NewEndpoint(
		suite.kubeEndpoints,
		addressB,
		&portB,
		Readiness(true),
	)
	assert.NoError(suite.T(), err)

	cases := []struct {
		baseE     *Endpoint
		comparedE *Endpoint
		expected  bool
	}{
		{suite.endpointA, suite.endpointA, true},
		{suite.endpointA, suite.endpointNonPod, false},
		{suite.endpointNonPod, suite.endpointA, false},
		{suite.endpointNonPod, suite.endpointNonPod, true},
		{suite.endpointA, suite.endpointA, true},
		{endpointA, suite.endpointA, true},
		{suite.endpointA, endpointA, true},
		{endpointB, endpointA, false},
		{endpointA, endpointB, false},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, c.baseE.isEqualTarget(c.comparedE))
	}
}

func (suite *EndpointTestSuite) TestEquals() {
	address := suite.kubeEndpoints.Subsets[0].Addresses[0].DeepCopy() // deep copy to get different targetref pointers
	port := suite.kubeEndpoints.Subsets[0].Ports[1]
	endpointA, err := NewEndpoint(
		suite.kubeEndpoints,
		address,
		&port,
		Readiness(false),
	)
	assert.NoError(suite.T(), err)

	addressB := suite.kubeEndpoints.Subsets[0].Addresses[1].DeepCopy() // deep copy to get different targetref pointers
	portB := suite.kubeEndpoints.Subsets[0].Ports[0]
	endpointB, err := NewEndpoint(
		suite.kubeEndpoints,
		addressB,
		&portB,
		Readiness(true),
	)
	assert.NoError(suite.T(), err)
	endpointBNotReady, err := NewEndpoint(
		suite.kubeEndpoints,
		addressB,
		&portB,
		Readiness(false),
	)
	assert.NoError(suite.T(), err)

	cases := []struct {
		baseE     *Endpoint
		comparedE *Endpoint
		expected  bool
	}{
		{suite.endpointA, suite.endpointA, true},
		{suite.endpointA, suite.endpointNonPod, false},
		{suite.endpointNonPod, suite.endpointA, false},
		{suite.endpointNonPod, suite.endpointNonPod, true},
		{suite.endpointA, suite.endpointA, true},
		{endpointA, suite.endpointA, true},
		{suite.endpointA, endpointA, true},
		{endpointB, endpointA, false},
		{endpointA, endpointB, false},
		{endpointBNotReady, endpointB, false},
		{endpointB, endpointB, true},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, c.baseE.Equals(c.comparedE))
	}
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestEndpointSuite(t *testing.T) {
	suite.Run(t, new(EndpointTestSuite))
}
