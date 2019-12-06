package v1alpha1

import (
	"testing"

	"github.com/Demonware/balanced/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type MapServiceManagerTestSuite struct {
	suite.Suite
	sm *MapServiceManager

	kubeSvcA *api.Service
	svcAA    *Service
	svcAB    *Service
	kubeSvcB *api.Service
	svcBA    *Service
	svcBB    *Service

	kubeEndpoints *api.Endpoints
	testLogger    util.Logger
}

func (suite *MapServiceManagerTestSuite) SetupTest() {
	suite.testLogger = util.NewNamedLogger("testLogger", 1)
	suite.sm = NewMapServiceManager(suite.testLogger)

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
	endpoints, err := ConvertKubeAPIV1EndpointsToEndpoints(suite.kubeEndpoints)
	if err != nil {
		suite.T().Errorf("cannot convert endpoints for testing")
	}

	suite.kubeSvcB = &api.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "baz",
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
	suite.svcAA, err = NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	_, err = suite.svcAA.UpsertEndpoints(endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints for testing")
	}

	suite.svcAB, err = NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[1])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	suite.svcBA, err = NewService(suite.kubeSvcB, suite.kubeSvcB.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	suite.svcBB, err = NewService(suite.kubeSvcB, suite.kubeSvcB.Spec.Ports[1])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
}

func (suite *MapServiceManagerTestSuite) TestExists() {
	sm := NewMapServiceManager(suite.testLogger)
	assert.False(suite.T(), sm.Exists(suite.svcAA))
	assert.NoError(suite.T(), sm.Add(suite.svcAA))
	assert.True(suite.T(), sm.Exists(suite.svcAA))
}

func (suite *MapServiceManagerTestSuite) TestResolveByKubeMetaNamespaceKey() {
	sm := NewMapServiceManager(suite.testLogger)
	svcs, err := sm.ResolveByKubeMetaNamespaceKey("foo/bar")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), svcs, 0)
	assert.NoError(suite.T(), sm.Add(suite.svcAA))
	assert.NoError(suite.T(), sm.Add(suite.svcAB))
	assert.NoError(suite.T(), sm.Add(suite.svcBA))

	svcs, err = sm.ResolveByKubeMetaNamespaceKey("foo/bar")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), svcs, 2)

	svcs, err = sm.ResolveByKubeMetaNamespaceKey("foo/nonexistent")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), svcs, 0)

	svcs, err = sm.ResolveByKubeMetaNamespaceKey("foo/baz")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), svcs, 1)
	assert.Equal(suite.T(), suite.svcBA.ID(), svcs[0].ID())
}

func (suite *MapServiceManagerTestSuite) TestNoLongerExistentServiceKeys() {
	cases := []struct {
		curServices         []*Service
		existingServices    []*Service
		expectedServiceKeys []string
	}{
		{[]*Service{}, []*Service{}, []string{}},
		{
			[]*Service{},
			[]*Service{suite.svcAA, suite.svcAB},
			[]string{suite.svcAA.ID(), suite.svcAB.ID()},
		},
		{
			[]*Service{suite.svcAA, suite.svcAB},
			[]*Service{},
			[]string{},
		},
		{
			[]*Service{suite.svcAA, suite.svcAB},
			[]*Service{suite.svcAA, suite.svcAB, suite.svcBA, suite.svcBB},
			[]string{suite.svcBA.ID(), suite.svcBB.ID()},
		},
	}
	for _, c := range cases {
		diffKeys := noLongerExistentServiceKeys(c.curServices, c.existingServices)
		assert.EqualValues(
			suite.T(),
			c.expectedServiceKeys,
			diffKeys,
		)
	}
}

func (suite *MapServiceManagerTestSuite) TestRemoveExistingServicesNoLongerCurrent() {
	var onServiceHandlerCalled int
	onServiceHandlerCalledFn := func(prevSvc *Service, svc *Service) {
		assert.NotNil(suite.T(), prevSvc)
		assert.Nil(suite.T(), svc)
		onServiceHandlerCalled++
	}

	sm := NewMapServiceManager(suite.testLogger)
	assert.NoError(suite.T(), sm.Add(suite.svcAA))

	cases := []struct {
		curServices      []*Service
		existingServices []*Service
		expectedErr      bool
	}{
		{[]*Service{}, []*Service{}, false},
		{
			[]*Service{suite.svcAB, suite.svcBB},
			[]*Service{suite.svcAA, suite.svcAB, suite.svcBB},
			false,
		},
		{
			[]*Service{suite.svcAA},
			[]*Service{suite.svcAA, suite.svcAB},
			true,
		},
	}
	for _, c := range cases {
		err := sm.RemoveExistingServicesNoLongerCurrent(c.curServices, c.existingServices, onServiceHandlerCalledFn)
		assert.Equal(
			suite.T(),
			c.expectedErr,
			err != nil,
		)
	}
	assert.Equal(suite.T(), 1, onServiceHandlerCalled)
}

func (suite *MapServiceManagerTestSuite) TestGet() {
	sm := NewMapServiceManager(suite.testLogger)
	svc, err := sm.Get("test")
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), svc)

	assert.NoError(suite.T(), sm.Add(suite.svcAA))

	svc, err = sm.Get(suite.svcAA.ID())
	assert.Equal(suite.T(), suite.svcAA, svc)
}

func (suite *MapServiceManagerTestSuite) TestList() {
	sm := NewMapServiceManager(suite.testLogger)
	assert.Len(suite.T(), sm.List(), 0)
	assert.NoError(suite.T(), sm.Add(suite.svcAA))
	assert.NoError(suite.T(), sm.Add(suite.svcAB))
	assert.NoError(suite.T(), sm.Add(suite.svcBA))

	svcs := sm.List()
	expectedList := []*Service{suite.svcAA, suite.svcAB, suite.svcBA}
	for _, expectedSvc := range expectedList {
		contains := false
		for _, svc := range svcs {
			if expectedSvc.ID() == svc.ID() {
				contains = true
				break
			}
		}
		if !contains {
			suite.T().Error("expectedService not in list")
		}
	}
	assert.Len(suite.T(), svcs, 3)
}

func (suite *MapServiceManagerTestSuite) TestListWithAddresses() {
	sm := NewMapServiceManager(suite.testLogger)
	assert.Len(suite.T(), sm.ListServicesWithAddress(), 0)
	assert.NoError(suite.T(), sm.Add(suite.svcAA))
	assert.NoError(suite.T(), sm.Add(suite.svcAB))
	assert.NoError(suite.T(), sm.Add(suite.svcBA))

	svcs := sm.ListServicesWithAddress()
	expectedList := []*Service{suite.svcAA, suite.svcAB}
	for _, expectedSvc := range expectedList {
		contains := false
		for _, svc := range svcs {
			if expectedSvc.ID() == svc.ID() {
				contains = true
				break
			}
		}
		if !contains {
			suite.T().Error("expectedService not in list")
		}
	}
	assert.Len(suite.T(), svcs, 2)
}

func (suite *MapServiceManagerTestSuite) TestAdd() {
	var onServiceHandlerCalled int
	onServiceHandlerCalledFn := func(prevSvc *Service, svc *Service) {
		assert.Nil(suite.T(), prevSvc)
		assert.NotNil(suite.T(), svc)
		onServiceHandlerCalled++
	}

	sm := NewMapServiceManager(suite.testLogger)
	assert.Len(suite.T(), sm.List(), 0)
	assert.NoError(suite.T(), sm.Add(suite.svcAA))
	assert.NoError(suite.T(), sm.Add(suite.svcAB, onServiceHandlerCalledFn))
	assert.NoError(suite.T(), sm.Add(suite.svcBA, onServiceHandlerCalledFn, onServiceHandlerCalledFn))

	assert.Len(suite.T(), sm.List(), 3)
	assert.Equal(suite.T(), 3, onServiceHandlerCalled)

	// adding existing svc
	assert.Error(suite.T(), sm.Add(suite.svcAB, onServiceHandlerCalledFn))
	assert.Len(suite.T(), sm.List(), 3)
	assert.Equal(suite.T(), 3, onServiceHandlerCalled)
}

func (suite *MapServiceManagerTestSuite) TestUpdate() {
	var onServiceHandlerCalled int
	onServiceHandlerCalledFn := func(prevSvc *Service, svc *Service) {
		assert.NotNil(suite.T(), prevSvc)
		assert.NotNil(suite.T(), svc)
		onServiceHandlerCalled++
	}

	sm := NewMapServiceManager(suite.testLogger)
	assert.NoError(suite.T(), sm.Add(suite.svcAA))

	_, _, err := sm.Update(suite.svcAB)
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), 0, onServiceHandlerCalled)

	updated, prevSvc, err := sm.Update(suite.svcAA, onServiceHandlerCalledFn)
	assert.False(suite.T(), updated)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), prevSvc, suite.svcAA)
	assert.Equal(suite.T(), 1, onServiceHandlerCalled)

	updatedKubeSvcA := suite.kubeSvcA.DeepCopy()
	updatedKubeSvcA.ObjectMeta.Annotations = map[string]string{"test": "test"}
	updatedSvcAA, err := NewService(updatedKubeSvcA, updatedKubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	updated, prevSvc, err = sm.Update(updatedSvcAA, onServiceHandlerCalledFn)
	assert.True(suite.T(), updated)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), prevSvc, suite.svcAA)
	assert.Equal(suite.T(), 2, onServiceHandlerCalled)
}

func (suite *MapServiceManagerTestSuite) TestRemove() {
	var onServiceHandlerCalled int
	onServiceHandlerCalledFn := func(prevSvc *Service, svc *Service) {
		assert.NotNil(suite.T(), prevSvc)
		assert.Nil(suite.T(), svc)
		onServiceHandlerCalled++
	}
	sm := NewMapServiceManager(suite.testLogger)
	assert.Error(suite.T(), sm.Remove(suite.svcAA, onServiceHandlerCalledFn))
	assert.Equal(suite.T(), 0, onServiceHandlerCalled)

	assert.NoError(suite.T(), sm.Add(suite.svcAA))
	assert.NoError(suite.T(), sm.Remove(suite.svcAA, onServiceHandlerCalledFn, onServiceHandlerCalledFn))
	assert.Equal(suite.T(), 2, onServiceHandlerCalled)

}

func (suite *MapServiceManagerTestSuite) TestRemoveByID() {
	var onServiceHandlerCalled int
	onServiceHandlerCalledFn := func(prevSvc *Service, svc *Service) {
		assert.NotNil(suite.T(), prevSvc)
		assert.Nil(suite.T(), svc)
		onServiceHandlerCalled++
	}
	sm := NewMapServiceManager(suite.testLogger)
	assert.Error(suite.T(), sm.RemoveByID(suite.svcAA.ID(), onServiceHandlerCalledFn))
	assert.Equal(suite.T(), 0, onServiceHandlerCalled)

	assert.NoError(suite.T(), sm.Add(suite.svcAA))
	assert.NoError(suite.T(), sm.RemoveByID(suite.svcAA.ID(), onServiceHandlerCalledFn, onServiceHandlerCalledFn))
	assert.Equal(suite.T(), 2, onServiceHandlerCalled)
}

func (suite *MapServiceManagerTestSuite) TestRemoveServicesRecursively() {
	var onServiceChangeCalled = 0
	var onEndpointChangeCalled = 0

	var onServiceChangeCalledFn = func(prevSvc *Service, svc *Service) {
		assert.NotNil(suite.T(), prevSvc, "should only be a del operation, prevSvc should be specified")
		assert.Nil(suite.T(), svc, "svc should not be specified")
		onServiceChangeCalled++
	}
	var onEndpointChangeCalledFn = func(prevE *Endpoint, e *Endpoint) {
		assert.NotNil(suite.T(), prevE, "should only be a del operation, prevE should be specified")
		assert.Nil(suite.T(), e, "e should not be specified")
		onEndpointChangeCalled++
	}

	sm := NewMapServiceManager(suite.testLogger)
	assert.NoError(
		suite.T(),
		sm.RemoveServicesRecursively(
			[]*Service{},
			[]ServiceOnChangeHandler{onServiceChangeCalledFn},
			[]EndpointOnChangeHandler{onEndpointChangeCalledFn},
		),
	)
	assert.Equal(suite.T(), 0, onServiceChangeCalled)
	assert.Equal(suite.T(), 0, onEndpointChangeCalled)
	svcAA, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	svcAB, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[1])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	endpoints, err := ConvertKubeAPIV1EndpointsToEndpoints(suite.kubeEndpoints)
	if err != nil {
		suite.T().Errorf("cannot convert endpoints for testing")
	}
	_, err = svcAA.UpsertEndpoints(endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints for testing")
	}
	_, err = svcAB.UpsertEndpoints(endpoints)
	if err != nil {
		suite.T().Errorf("cannot upsert endpoints for testing")
	}
	assert.Error(
		suite.T(),
		sm.RemoveServicesRecursively(
			[]*Service{svcAB},
			[]ServiceOnChangeHandler{onServiceChangeCalledFn},
			[]EndpointOnChangeHandler{onEndpointChangeCalledFn},
		),
	)
	assert.Equal(suite.T(), 0, onServiceChangeCalled)
	assert.Equal(suite.T(), 2, onEndpointChangeCalled)

	err = sm.Add(svcAA)
	if err != nil {
		suite.T().Errorf("cannot add new service for testing")
	}
	err = sm.Add(svcAB)
	if err != nil {
		suite.T().Errorf("cannot add new service for testing")
	}
	assert.NoError(
		suite.T(),
		sm.RemoveServicesRecursively(
			[]*Service{svcAA, svcAB},
			[]ServiceOnChangeHandler{onServiceChangeCalledFn},
			[]EndpointOnChangeHandler{onEndpointChangeCalledFn},
		),
	)
	assert.Equal(suite.T(), 2, onServiceChangeCalled)
	assert.Equal(suite.T(), 4, onEndpointChangeCalled)
}

func (suite *MapServiceManagerTestSuite) TestUpdateServicesRecursively() {
	var onServiceAddCalled = 0
	var onEndpointAddCalled = 0
	var onServiceChangeCalled = 0
	var onEndpointChangeCalled = 0

	var onServiceAddCalledFn = func(prevSvc *Service, svc *Service) {
		assert.Nil(suite.T(), prevSvc, "should only be an add operation, prevSvc should not be specified")
		assert.NotNil(suite.T(), svc, "svc should be specified")
		onServiceAddCalled++
	}
	var onEndpointAddCalledFn = func(prevE *Endpoint, e *Endpoint) {
		assert.Nil(suite.T(), prevE, "should only be an add operation, prevE should not be specified")
		assert.NotNil(suite.T(), e, "e should be specified")
		onEndpointAddCalled++
	}

	var onServiceChangeCalledFn = func(prevSvc *Service, svc *Service) {
		assert.NotNil(suite.T(), prevSvc, "prevSvc should be specified")
		assert.NotNil(suite.T(), svc, "svc should not be specified")
		onServiceChangeCalled++
	}
	var onEndpointChangeCalledFn = func(prevE *Endpoint, e *Endpoint) {
		assert.NotNil(suite.T(), prevE, "prevE should be specified")
		assert.NotNil(suite.T(), e, "e should not be specified")
		onEndpointChangeCalled++
	}

	sm := NewMapServiceManager(suite.testLogger)
	assert.NoError(
		suite.T(),
		sm.UpdateServicesRecursively(
			[]*Service{},
			[]*Endpoint{},
			[]ServiceOnChangeHandler{onServiceAddCalledFn},
			[]EndpointOnChangeHandler{onEndpointAddCalledFn},
		),
	)
	assert.Equal(suite.T(), 0, onServiceAddCalled)
	assert.Equal(suite.T(), 0, onEndpointAddCalled)

	svcAA, err := NewService(suite.kubeSvcA, suite.kubeSvcA.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	kubeSvcAWithAnnotations := suite.kubeSvcA.DeepCopy()
	kubeSvcAWithAnnotations.ObjectMeta.Annotations = map[string]string{"test": "test"}
	svcAAWithAnnotations, err := NewService(kubeSvcAWithAnnotations, kubeSvcAWithAnnotations.Spec.Ports[0])
	if err != nil {
		suite.T().Errorf("cannot create new service for testing")
	}
	endpoints, err := ConvertKubeAPIV1EndpointsToEndpoints(suite.kubeEndpoints)
	if err != nil {
		suite.T().Errorf("cannot convert endpoints for testing")
	}
	assert.NoError(
		suite.T(),
		sm.UpdateServicesRecursively(
			[]*Service{svcAA},
			endpoints,
			[]ServiceOnChangeHandler{onServiceAddCalledFn},
			[]EndpointOnChangeHandler{onEndpointAddCalledFn},
		),
	)
	assert.Equal(suite.T(), 1, onServiceAddCalled)
	assert.Equal(suite.T(), 2, onEndpointAddCalled)
	assert.NoError(
		suite.T(),
		sm.UpdateServicesRecursively(
			[]*Service{svcAA},
			endpoints,
			[]ServiceOnChangeHandler{onServiceChangeCalledFn},
			[]EndpointOnChangeHandler{onEndpointChangeCalledFn},
		),
	)
	assert.Equal(suite.T(), 1, onServiceChangeCalled)
	assert.Equal(suite.T(), 2, onEndpointChangeCalled)
	assert.NoError(
		suite.T(),
		sm.UpdateServicesRecursively(
			[]*Service{svcAAWithAnnotations},
			endpoints,
			[]ServiceOnChangeHandler{onServiceChangeCalledFn},
			[]EndpointOnChangeHandler{onEndpointChangeCalledFn},
		),
	)
	assert.Equal(suite.T(), 2, onServiceChangeCalled)
	assert.Equal(suite.T(), 4, onEndpointChangeCalled)
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestMapServiceManagerSuite(t *testing.T) {
	suite.Run(t, new(MapServiceManagerTestSuite))
}
