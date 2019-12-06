package main

import (
	"net"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type HostPodCacheTestSuite struct {
	suite.Suite
}

func (suite *HostPodCacheTestSuite) SetupTest() {
}

func (suite *HostPodCacheTestSuite) TestSameStringSlice() {
	cases := []struct {
		a          []string
		b          []string
		expectBool bool
	}{
		{nil, nil, true},
		{[]string{}, []string{}, true},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{}, false},
		{[]string{}, []string{"a", "b"}, false},
		{[]string{"a"}, nil, false},
		{nil, []string{"a"}, false},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a", "b", "c"}, []string{"c", "a", "b"}, true},
		{[]string{"a", "b", "c"}, []string{"c", "b", "a"}, true},
		{[]string{"a"}, []string{"b"}, false},
		{[]string{"b"}, []string{"a"}, false},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expectBool, hasIdenticalElements(c.a, c.b))
	}
}

func (suite *HostPodCacheTestSuite) TestConvertNetIPToStringSlice() {
	cases := []struct {
		ips      []net.IP
		expected []string
	}{
		{nil, []string{}},
		{[]net.IP{}, []string{}},
		{[]net.IP{net.ParseIP("1.1.1.1")}, []string{"1.1.1.1"}},
		{[]net.IP{net.ParseIP("1.1.1.1"), net.ParseIP("2.2.2.2")}, []string{"1.1.1.1", "2.2.2.2"}},
	}
	for _, c := range cases {
		assert.True(suite.T(), hasIdenticalElements(c.expected, convertNetIPToStringSlice(c.ips)))
	}
}

func (suite *HostPodCacheTestSuite) TestContainerIDString() {
	containerA := "docker://test"
	cases := []struct {
		containerID *string
		expected    string
	}{
		{nil, ""},
		{&containerA, containerA},
	}
	for _, c := range cases {
		assert.Equal(suite.T(), c.expected, containerIDString(c.containerID))
	}
}

func (suite *HostPodCacheTestSuite) TestHasChanged() {
	podA := &v1.Pod{
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
					ContainerID: "docker://abcdefg",
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	podANewContainerID := &v1.Pod{
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
					ContainerID: "docker://new",
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	hp1 := NewHostPod(podA)
	hp2 := NewHostPod(podANewContainerID)
	hpc := NewThreadsafeHostPodCache()
	assert.True(suite.T(), hpc.HasChanged(hp1))
	hpc.Set(hp1)
	assert.False(suite.T(), hpc.HasChanged(hp1))
	assert.True(suite.T(), hpc.HasChanged(hp2))
	hpc.Delete(hp1)
	assert.True(suite.T(), hpc.HasChanged(hp1))
	assert.True(suite.T(), hpc.HasChanged(hp2))
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestHostPodCacheSuite(t *testing.T) {
	suite.Run(t, new(HostPodCacheTestSuite))
}
