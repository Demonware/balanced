package main

import (
	"fmt"
	"net"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/Demonware/balanced/mocks"

	"github.com/Demonware/balanced/pkg/apis/balanced/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type HostPodTestSuite struct {
	suite.Suite
}

func (suite *HostPodTestSuite) SetupTest() {
}

func (suite *HostPodTestSuite) TestID() {
	podA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	hostPod := NewHostPod(podA)
	assert.Equal(suite.T(), "foo/bar", hostPod.ID())
}

func (suite *HostPodTestSuite) TestUpdatePod() {
	podA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	podB := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busyboxUpdated",
				},
			},
		},
	}
	podC := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busyboxUpdated",
				},
			},
		},
	}
	hostPod := NewHostPod(podA)
	assert.Equal(suite.T(), podA, hostPod.Pod())
	hostPod.UpdatePod(podB)
	assert.Equal(suite.T(), podB, hostPod.Pod())
	assert.Error(suite.T(), hostPod.UpdatePod(podC))
}

func (suite *HostPodTestSuite) TestAddresses() {
	podA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	kubeSvcA := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Ports: []v1.ServicePort{
				v1.ServicePort{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
	}
	kubeSvcB := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
			Namespace: "foo",
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Ports: []v1.ServicePort{
				v1.ServicePort{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
				v1.ServicePort{
					Protocol:   "TCP",
					Port:       int32(443),
					TargetPort: intstr.FromInt(443),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					v1.LoadBalancerIngress{
						IP: "192.168.1.100",
					},
				},
			},
		},
	}
	kubeSvcC := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bum",
			Namespace: "foo",
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Ports: []v1.ServicePort{
				v1.ServicePort{
					Protocol:   "TCP",
					Port:       int32(80),
					TargetPort: intstr.FromInt(80),
					NodePort:   int32(34151),
				},
			},
			ClusterIP: "100.64.0.111",
		},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					v1.LoadBalancerIngress{
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

	hostPod := NewHostPod(podA)
	assert.NoError(suite.T(), hostPod.Set(svcA))
	assert.NoError(suite.T(), hostPod.Set(svcB))
	assert.NoError(suite.T(), hostPod.Set(svcC))
	assert.NoError(suite.T(), hostPod.Set(svcD))

	expectedIPs := []net.IP{
		net.ParseIP("192.168.1.2"),
		net.ParseIP("192.168.1.100"),
		net.ParseIP("192.168.1.100"),
	}
	ips := hostPod.Addresses()

	for _, expectedIP := range expectedIPs {
		assert.Contains(suite.T(), ips, expectedIP)
	}
	assert.Equal(suite.T(), len(expectedIPs), len(ips))
}

func (suite *HostPodTestSuite) TestContainerID() {
	podA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	containerID := "docker://abcdefg"
	podB := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: containerID,
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	podC := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: containerID,
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{},
					},
				},
				{
					ContainerID: "",
					State: v1.ContainerState{
						Waiting: &v1.ContainerStateWaiting{},
					},
				},
			},
		},
	}
	podD := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: "",
					State: v1.ContainerState{
						Waiting: &v1.ContainerStateWaiting{},
					},
				},
				{
					ContainerID: containerID,
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	podE := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
		Status: v1.PodStatus{
			ContainerStatuses: []v1.ContainerStatus{
				{
					ContainerID: "",
					State: v1.ContainerState{
						Waiting: &v1.ContainerStateWaiting{},
					},
				},
			},
		},
	}
	cases := []struct {
		pod                 *v1.Pod
		expectedContainerID *string
	}{
		{podA, nil},
		{podB, &containerID},
		{podC, &containerID},
		{podD, &containerID},
		{podE, nil},
	}
	for _, c := range cases {
		hostPod := NewHostPod(c.pod)
		assert.EqualValues(suite.T(), c.expectedContainerID, hostPod.ContainerID())
	}
}

func (suite *HostPodTestSuite) TestIsValidPod() {
	podA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	hostNetworkPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			HostNetwork: true,
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	ignorePodA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				ignoreAnnotation: "true",
			},
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	ignorePodB := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				ignoreAnnotation: "",
			},
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	legacyIgnorePod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				legacyIgnoreAnnotation: "a", // any val should match
			},
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}

	cases := []struct {
		pod      *v1.Pod
		expected bool
	}{
		{podA, true},
		{hostNetworkPod, false},
		{ignorePodA, false},
		{ignorePodB, false},
		{legacyIgnorePod, false},
	}
	for _, c := range cases {
		hostPod := NewHostPod(c.pod)
		assert.Equal(suite.T(), c.expected, hostPod.IsValidPod())
	}
}

func (suite *HostPodTestSuite) TestRemove() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockSm := mocks.NewMockServiceManager(mockCtrl)
	podA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	hostPod := &HostPod{
		pod: podA,
		sm:  mockSm,
	}

	mockSm.EXPECT().Remove(nil).Return(nil).Times(1)
	mockSm.EXPECT().Remove(nil).Return(fmt.Errorf("test")).Times(1)

	assert.NoError(suite.T(), hostPod.Remove(nil))
	assert.Error(suite.T(), hostPod.Remove(nil))
}

func (suite *HostPodTestSuite) TestUpsert() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockSm := mocks.NewMockServiceManager(mockCtrl)
	podA := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bar",
			Namespace: "foo",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "busybox",
					Image: "busybox",
				},
			},
		},
	}
	hostPod := &HostPod{
		pod: podA,
		sm:  mockSm,
	}

	mockSm.EXPECT().Exists(nil).Return(true).Times(2)
	mockSm.EXPECT().Update(nil).Return(false, nil, fmt.Errorf("test error")).Times(1)
	mockSm.EXPECT().Update(nil).Return(false, nil, nil).Times(1)
	mockSm.EXPECT().Exists(nil).Return(false).Times(2)
	mockSm.EXPECT().Add(nil).Return(nil).Times(1)
	mockSm.EXPECT().Add(nil).Return(fmt.Errorf("test error")).Times(1)

	assert.Error(suite.T(), hostPod.Set(nil))
	assert.NoError(suite.T(), hostPod.Set(nil))
	assert.NoError(suite.T(), hostPod.Set(nil))
	assert.Error(suite.T(), hostPod.Set(nil))
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestHostPodSuite(t *testing.T) {
	suite.Run(t, new(HostPodTestSuite))
}
