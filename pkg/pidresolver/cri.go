package pidresolver

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/util"
	"os"
)

const (
	CRIResolver ResolverType = "cri"
)

type ContainerInfo struct {
	PID int `json:"pid"`
}

type CRIPIDResolver struct {
	runtimeEndpoint string
}

func NewCRIPIDResolver(runtimeEndpoint string) *CRIPIDResolver {
	return &CRIPIDResolver{
		runtimeEndpoint: runtimeEndpoint,
	}
}

func (c *CRIPIDResolver) getRuntimeClient() (*grpc.ClientConn, pb.RuntimeServiceClient, error) {
	addr, dialer, err := util.GetAddressAndDialer(c.runtimeEndpoint)
	if err != nil {
		return nil, nil, err
	}
	if _, err := os.Stat(addr); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("CRI runtime endpoint does not exist")
	}
	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithDialer(dialer),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect: %v", err)
	}

	return conn, pb.NewRuntimeServiceClient(conn), nil
}

func (c *CRIPIDResolver) withRuntimeClient(f func(pb.RuntimeServiceClient) error) error {
	conn, runtimeClient, err := c.getRuntimeClient()
	if err != nil {
		return err
	}
	defer func() {
		// don't really do anything about that error, so just ignoring it explicitly
		_ = conn.Close()
	}()
	return f(runtimeClient)
}

func (c *CRIPIDResolver) GetPID(containerID string) (pid int, err error) {
	err = c.withRuntimeClient(func(client pb.RuntimeServiceClient) error {
		var (
			r   *pb.ContainerStatusResponse
			err error
		)
		request := &pb.ContainerStatusRequest{
			ContainerId: containerID,
			Verbose:     true, // needed to get .Info map from response
		}
		r, err = client.ContainerStatus(context.Background(), request)
		if err != nil {
			return err
		}
		containerInfoJsonStr, ok := r.Info["info"]
		if !ok {
			return fmt.Errorf("cannot find info from ContainerStatus response")
		}
		var containerInfo = &ContainerInfo{}
		if err := json.Unmarshal([]byte(containerInfoJsonStr), containerInfo); err != nil {
			return fmt.Errorf("error unmarshaling ContainerStatus.Info response")
		}
		pid = containerInfo.PID
		return nil
	})
	return
}

func (c *CRIPIDResolver) ResolverType() ResolverType {
	return CRIResolver
}
