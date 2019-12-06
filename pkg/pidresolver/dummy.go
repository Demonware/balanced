package pidresolver

type DummyPIDResolver struct {
	pid int
}

const (
	DummyResolver ResolverType = "dummy"
)

func NewDummyPIDResolver(pid int) *DummyPIDResolver {
	return &DummyPIDResolver{
		pid: pid,
	}
}
func (d *DummyPIDResolver) GetPID(containerID string) (int, error) {
	return d.pid, nil
}

func (d *DummyPIDResolver) ResolverType() ResolverType {
	return DummyResolver
}
