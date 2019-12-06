package pidresolver

type ConfigResolver struct {
	Containerd *ConfigContainerResolver `json:"containerd,omitempty"`
}

type ConfigContainerResolver struct {
	Endpoint string `json:"endpoint"`
}
