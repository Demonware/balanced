package pidresolver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/patrickmn/go-cache"

	dockerclient "github.com/docker/docker/client"
)

const (
	DockerResolver ResolverType = "docker"
)

// DockerPIDResolver resolves the PID of a container using the docker daemon
type DockerPIDResolver struct {
	dockerClient dockerclient.APIClient

	// thread-safe cache
	containerPidCache *cache.Cache
}

func NewDockerPIDResolver(docker dockerclient.APIClient) *DockerPIDResolver {
	return &DockerPIDResolver{
		dockerClient: docker,
		// TODO: make it configurable
		containerPidCache: cache.New(24*time.Hour, 24*time.Hour),
	}
}

func (d *DockerPIDResolver) GetPID(containerID string) (pid int, err error) {
	// Resolving PID using docker daemon is an expensive call, so we make use of a cache
	defer func() {
		if pid > 0 {
			d.containerPidCache.Set(containerID, pid, cache.DefaultExpiration)
		}
	}()
	containerID = strings.TrimPrefix(containerID, "docker://")
	if cachedPID, exists := d.containerPidCache.Get(containerID); exists {
		pid = cachedPID.(int)
	}
	if pid <= 0 {
		var (
			containerSpec types.ContainerJSON
			body          []byte
		)
		containerSpec, body, err = d.dockerClient.ContainerInspectWithRaw(
			context.Background(),
			containerID,
			false,
		)
		if err != nil {
			err = fmt.Errorf(
				"failed to get docker container spec for %s due to %s: %s",
				containerID, err.Error(), body)
			return
		}
		pid = containerSpec.State.Pid
		if pid == 0 {
			err = fmt.Errorf(
				"cannot get endpoint/pod PID. Container %s does not exist in Docker",
				containerID)
			return
		}
	}
	return
}

func (d *DockerPIDResolver) ResolverType() ResolverType {
	return DockerResolver
}
