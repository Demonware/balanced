package pidresolver

type ResolverType string

type PIDResolver interface {
	GetPID(string) (int, error)
	ResolverType() ResolverType
}
