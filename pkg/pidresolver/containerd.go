package pidresolver

import (
	"strings"
)

const (
	ContainerdResolver ResolverType = "containerd"
)

type ContainerdPIDResolver struct {
	criPIDResolver PIDResolver
}

func NewContainerdPIDResolver(runtimeEndpoint string) *ContainerdPIDResolver {
	criPIDResolver := NewCRIPIDResolver(runtimeEndpoint)
	return &ContainerdPIDResolver{
		criPIDResolver: criPIDResolver,
	}
}

func (c *ContainerdPIDResolver) GetPID(containerID string) (int, error) {
	containerID = strings.TrimPrefix(containerID, "containerd://")
	return c.criPIDResolver.GetPID(containerID)
}

func (c *ContainerdPIDResolver) ResolverType() ResolverType {
	return ContainerdResolver
}
