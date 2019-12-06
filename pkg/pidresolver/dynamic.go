package pidresolver

import (
	"fmt"
	"github.com/golang/glog"
	"strings"
)

const (
	DynamicResolver ResolverType = "dynamic"
)

type DynamicPIDResolver struct {
	resolverMap map[ResolverType]PIDResolver
}

func NewDynamicPIDResolver(resolvers ...PIDResolver) (*DynamicPIDResolver, error) {
	d := &DynamicPIDResolver{
		resolverMap: make(map[ResolverType]PIDResolver),
	}
	for _, resolver := range resolvers {
		if _, exists := d.resolverMap[resolver.ResolverType()]; exists {
			return nil, fmt.Errorf("cannot register more than one of the same resolver type: %s",
				resolver.ResolverType())
		}
		d.resolverMap[resolver.ResolverType()] = resolver
	}
	return d, nil
}

func (d *DynamicPIDResolver) GetPID(containerID string) (int, error) {
	for prefix, pidR := range d.resolverMap {
		if strings.HasPrefix(containerID, fmt.Sprintf("%s://", prefix)) {
			glog.V(4).Infof("Using ResolverType: %s", prefix)
			return pidR.GetPID(containerID)
		}
	}
	return -1, fmt.Errorf(
		"cannot find PID resolver with the same prefix as containerID: %s", containerID)
}

func (d *DynamicPIDResolver) ResolverType() ResolverType {
	return DynamicResolver
}
