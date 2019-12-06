package health

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/glog"
)

type HealthPublisher struct {
	BindAddress   string
	Path          string
	Port          string
	HealthManager *HealthManager
}

func (hp *HealthPublisher) HealthHandler(w http.ResponseWriter, req *http.Request) {
	if hp.HealthManager.Healthy() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\n"))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Unhealthy\n"))
	}
}

func (hp *HealthPublisher) ChecksHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hp.HealthManager.Checks)
}

func (hp *HealthPublisher) Run(stopc <-chan struct{}) {
	glog.Info("Starting Health Publisher")

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", hp.BindAddress, hp.Port),
		Handler: http.DefaultServeMux,
	}

	http.HandleFunc(hp.Path, hp.HealthHandler)
	http.HandleFunc("/checks", hp.ChecksHandler)

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil {
			glog.Errorf("HealthPublisher HTTP server error: %s", err.Error())
		}
	}()

	<-stopc
	glog.Info("Stopping Health Publisher")
}

func NewHealthPublisher(bindAddr string, path string, port string, healthManager *HealthManager) *HealthPublisher {
	return &HealthPublisher{
		BindAddress:   bindAddr,
		Path:          path,
		Port:          port,
		HealthManager: healthManager,
	}
}
