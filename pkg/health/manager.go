package health

type HealthManager struct {
	Checks []HealthCheck
}

func NewHealthManager(c ...HealthCheck) *HealthManager {
	hm := &HealthManager{}
	if len(c) > 0 {
		hm.RegisterCheck(c...)
	}
	return hm
}

func (hm *HealthManager) RegisterCheck(c ...HealthCheck) {
	// TODO: add duplicate exporter detection
	hm.Checks = append(hm.Checks, c...)
}

func (hm *HealthManager) Status() (healthy, unhealthy []HealthCheck) {
	for _, c := range hm.Checks {
		if c.Healthy() {
			healthy = append(healthy, c)
		} else {
			unhealthy = append(unhealthy, c)
		}
	}
	return
}

func (hm *HealthManager) Healthy() bool {
	_, unhealthy := hm.Status()
	if len(unhealthy) > 0 {
		return false
	}
	return true
}
