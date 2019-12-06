package main

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/Demonware/balanced/pkg/pidresolver"

	dockerclient "github.com/docker/docker/client"
)

func pidResolver(config pidresolver.ConfigResolver, resolverType pidresolver.ResolverType) (pidresolver.PIDResolver, error) {
	glog.Infof("PIDResolver: %s", string(resolverType))
	switch resolverType {
	case pidresolver.DynamicResolver:
		dockerResolver, err := dockerPIDResolverFactory()
		if err != nil {
			return nil, err
		}
		containerdResolver := pidresolver.NewContainerdPIDResolver(config.Containerd.Endpoint)
		return pidresolver.NewDynamicPIDResolver(dockerResolver, containerdResolver)
	case pidresolver.DockerResolver:
		return dockerPIDResolverFactory()
	case pidresolver.ContainerdResolver:
		return pidresolver.NewContainerdPIDResolver(config.Containerd.Endpoint), nil
	case pidresolver.DummyResolver:
		return pidresolver.NewDummyPIDResolver(42), nil
	default:
		return nil, fmt.Errorf("specified PID Resolver does not exist")
	}
}

func dockerPIDResolverFactory() (pidresolver.PIDResolver, error) {
	dockerClient, err := dockerclient.NewEnvClient()
	if err != nil {
		return nil, err
	}
	return pidresolver.NewDockerPIDResolver(dockerClient), nil
}
