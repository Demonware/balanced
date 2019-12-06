package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/golang/glog"

	"github.com/Demonware/balanced/pkg/util"
	utilipvs "github.com/Demonware/balanced/pkg/util/ipvs"
)

var (
	loadKernelModules bool
	cidrStr           string
	interval          time.Duration
)

func parseCidrs(cidrsStr string) (ipNets []*net.IPNet, err error) {
	cidrs := strings.Split(cidrsStr, ",")
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		ipNets = append(ipNets, ipNet)
	}
	return
}

func ensureIptables(ipNets []*net.IPNet) error {
	iptablesHandle, err := iptables.New()
	if err != nil {
		return fmt.Errorf("failed to create iptables handle: %s", err)
	}

	err = utilipvs.EnsureIptables(iptablesHandle, ipNets)
	if err != nil {
		return fmt.Errorf("failed to setup iptables nat chain: %s", err)
	}
	glog.V(2).Infof("iptables rules have been synced")
	return nil
}

func iptablesRunFunc(ipNets []*net.IPNet) func(<-chan struct{}) {
	return func(stopc <-chan struct{}) {
		go func() {
			for {
				if err := ensureIptables(ipNets); err != nil {
					glog.Errorf("error ensuring iptables rules: %s", err)
				}
				time.Sleep(interval)
			}
		}()

		<-stopc
		glog.Infof("Stopping BalanceD Host controller")
	}
}

func realMain() (util.RunFunc, error) {
	if loadKernelModules {
		err := util.LoadKernelModule("ip_vs")
		if err != nil {
			return nil, err
		}
		utilipvs.LoadIpvsSchedulerKernelModulesIfExist()
	}

	ipNets, err := parseCidrs(cidrStr)
	if err != nil {
		return nil, fmt.Errorf("User supplied CIDRs are invalid (%s): %s", cidrStr, err.Error())
	}

	return iptablesRunFunc(ipNets), nil
}

func main() {
	flag.StringVar(&cidrStr, "cidr", "", "VIP CIDR(s) that this controller will care about. Use commas as delimiter. (eg. 10.49.248.0/24 or 10.49.248.0/24,10.50.20.0/24)")
	flag.BoolVar(&loadKernelModules, "load_kernel_modules", true, "Load required kernel modules on start-up of controller")
	flag.DurationVar(&interval, "interval", 1*time.Minute, "Interval to enforce iptables rules")
	flag.Parse()

	if cidrStr == "" {
		glog.Fatal("CIDR is required to be specified. Use -cidr")
	}

	runFunc, err := realMain()
	if err != nil {
		glog.Fatal(err)
	}
	var (
		stopc = make(chan struct{})
	)
	go func(f util.RunFunc) {
		f(stopc)
	}(runFunc)

	go func() {
		defer close(stopc)
		sigc := make(chan os.Signal)
		signal.Notify(sigc, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigc
		glog.Infof("Signal handled (%s), terminating application", sig)
	}()

	<-stopc
}
