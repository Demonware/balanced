package util

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/Demonware/balanced/pkg/health"
	"github.com/Demonware/balanced/pkg/metrics"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	Hostname        string
	ResyncPeriod    time.Duration
	BindAddress     string
	MetricsPath     string
	MetricsPort     string
	MetricsInterval time.Duration
	HealthPath      string
	HealthPort      string
	WatcherSyncDur  time.Duration
)

type RunFunc func(<-chan struct{})

type MainFunc func(*rest.Config, interface{}, *metrics.MetricsPublisher, *health.HealthManager) ([]RunFunc, error)

func Main(f MainFunc, cfg interface{}) {
	if err := realMain(f, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func loadConfig(path string, cfg interface{}) error {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bytes, cfg)
}

func realMain(f MainFunc, cfg interface{}) error {
	var (
		url               string
		kubeconfig        string
		configPath        string
		defaultKubeConfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	)
	flag.StringVar(&kubeconfig, "kubeconfig", defaultKubeConfig, "path to kubeconfig")
	flag.StringVar(&url, "url", "", "kubernetes master url")

	if cfg != nil {
		flag.StringVar(&configPath, "config", "", "path to config file")
	}

	flag.StringVar(&Hostname, "hostname", "", "hostname to assume")
	var err error
	if Hostname == "" {
		Hostname, err = os.Hostname()
		glog.Infof("-hostname not specified. Defaulting hostname to %s", Hostname)
		if err != nil {
			return err
		}
	}
	flag.DurationVar(&ResyncPeriod, "resync_period", 60*time.Second, "resync period for controllers")
	flag.DurationVar(&MetricsInterval, "metrics_interval", 10*time.Second, "interval to refresh metrics")
	flag.StringVar(&BindAddress, "bind_address", "", "Bind Address for HTTP server such as metrics and health")
	flag.StringVar(&MetricsPath, "metrics_path", "/metrics", "HTTP path to prometheus metrics endpoint")
	flag.StringVar(&MetricsPort, "metrics_port", "8123", "HTTP port for metrics server")

	flag.StringVar(&HealthPath, "health_path", "/healthz", "HTTP path to health endpoint")
	flag.StringVar(&HealthPort, "health_port", "8124", "HTTP port for health server")
	flag.DurationVar(&WatcherSyncDur, "watchersync_dur", 60*time.Second, "expected maximum duration for sync (used for healthchecks)")
	flag.Parse()

	var (
		metricsPublisher = metrics.NewMetricsPublisher(
			BindAddress, MetricsPath, MetricsPort, MetricsInterval)

		healthManager   = health.NewHealthManager()
		healthPublisher = health.NewHealthPublisher(
			BindAddress, HealthPath, HealthPort, healthManager)
	)

	if configPath != "" {
		if err := loadConfig(configPath, cfg); err != nil {
			return err
		}
	}

	config, err := clientcmd.BuildConfigFromFlags(url, kubeconfig)
	if err != nil {
		return err
	}
	runFuncs, err := f(config, cfg, metricsPublisher, healthManager)
	if err != nil {
		return err
	}

	runFuncs = append(runFuncs, []RunFunc{metricsPublisher.Run, healthPublisher.Run}...)

	var (
		stopc = make(chan struct{})
		wg    sync.WaitGroup
	)
	for _, f := range runFuncs {
		wg.Add(1)
		go func(f RunFunc) {
			defer wg.Done()
			f(stopc)
		}(f)
	}
	go func() {
		defer close(stopc)
		sigc := make(chan os.Signal)
		signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigc
		glog.Infof("Signal handled (%s), terminating application", sig)
	}()
	wg.Wait()
	return nil
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

// Difference returns the elements in a that aren't in b
func Difference(a, b []string) []string {
	mb := map[string]bool{}
	for _, x := range b {
		mb[x] = true
	}
	ab := []string{}
	for _, x := range a {
		if _, ok := mb[x]; !ok {
			ab = append(ab, x)
		}
	}
	return ab
}

func LoadKernelModule(name string) error {
	glog.V(2).Infof("Loading %s kernel module", name)
	if _, err := exec.Command("modprobe", name).CombinedOutput(); err != nil {
		return fmt.Errorf(
			"Failed to load %s kernel module: %s", name, err.Error(),
		)
	}
	return nil
}
