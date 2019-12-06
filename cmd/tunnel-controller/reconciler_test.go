package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TunnelReconcilerTestSuite struct {
	suite.Suite
}

func (suite *TunnelReconcilerTestSuite) SetupTest() {
}

func (suite *TunnelReconcilerTestSuite) TestIsValidEndpoint() {
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
							UID:       "test",
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
				NotReadyAddresses: []api.EndpointAddress{
					{
						IP:       "100.96.100.125",
						NodeName: &nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-dead-pod-1234",
							Namespace: "foo",
						},
					},
					{
						IP:       "100.96.100.126",
						NodeName: &nodeName,
						TargetRef: &api.ObjectReference{
							Kind:      "Pod",
							Name:      "i-am-a-dead-pod-12345",
							Namespace: "foo",
						},
					},
					{
						IP:       "100.96.100.127",
						NodeName: &nodeName,
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
	address := kubeEndpoints.Subsets[0].Addresses[0]
	port := kubeEndpoints.Subsets[0].Ports[1]
	endpointA, err := v1alpha1.NewEndpoint(
		kubeEndpoints,
		&address,
		&port,
		v1alpha1.Readiness(false),
	)
	if err != nil {
		suite.T().Errorf("cannot create endpoint A")
	}
	portB := kubeEndpoints.Subsets[0].Ports[0]
	endpointB, err := v1alpha1.NewEndpoint(
		kubeEndpoints,
		&address,
		&portB,
		v1alpha1.Readiness(false),
	)
	if err != nil {
		suite.T().Errorf("cannot create endpoint A")
	}
	kubeEndpointsNonPods := &api.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
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
	addressB := kubeEndpointsNonPods.Subsets[0].Addresses[1]
	portC := kubeEndpointsNonPods.Subsets[0].Ports[1]

	endpointNonPod, err := v1alpha1.NewEndpoint(
		kubeEndpoints,
		&addressB,
		&portC,
		v1alpha1.Readiness(true),
	)
	if err != nil {
		suite.T().Errorf("cannot create endpointNonPod")
	}

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

	svcA, err := v1alpha1.NewService(kubeSvcA, kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}

	endpointA.SetService(svcA)

	cases := []struct {
		endpoint   *v1alpha1.Endpoint
		hostname   string
		expectBool bool
	}{
		{endpointA, "test", true},
		{endpointA, "nomatch", false},
		{endpointB, "test", false},
		{endpointNonPod, "test", false},
		{endpointNonPod, "nomatch", false},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expectBool, isValidEndpoint(c.hostname, c.endpoint), c.hostname, c.endpoint.ID())
	}
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestTunnelReconcilerSuite(t *testing.T) {
	suite.Run(t, new(TunnelReconcilerTestSuite))
}
