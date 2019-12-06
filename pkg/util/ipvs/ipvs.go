package ipvs

import (
	"fmt"
	"net"

	"github.com/golang/glog"

	"github.com/coreos/go-iptables/iptables"
	"github.com/docker/libnetwork/ipvs"
	"github.com/Demonware/balanced/pkg/util"
)

const (
	IPTABLES_TABLE      = "raw"
	IPTABLES_PREROUTING = "PREROUTING"
	IPTABLES_CHAIN      = "IPVS-CONTROLLER"

	HighestRandomWeightScheduler = "hrw"
	MagLevScheduler              = "mh"
)

var (
	// TODO: make this as a config option
	SupportedIpvsSchedulers = []string{HighestRandomWeightScheduler, MagLevScheduler, ipvs.RoundRobin, ipvs.LeastConnection, ipvs.SourceHashing, ipvs.DestinationHashing}
)

// Service defines an IPVS service in its entirety.
type Service struct {
	*ipvs.Service
}

func NewIpvsService(svc *ipvs.Service) *Service {
	return &Service{svc}
}

func (s *Service) ID() string {
	return fmt.Sprintf("%s-%d-%d",
		s.Address.String(), s.Protocol, s.Port)
}

// Destination defines an IPVS destination (real server) in its
// entirety.
type Destination struct {
	*ipvs.Destination
}

func (d *Destination) ID() string {
	return fmt.Sprintf("%s-%d",
		d.Address.String(), d.Port)
}

func NewIpvsDestination(dst *ipvs.Destination) *Destination {
	return &Destination{dst}
}

func EnsureSysctlSettings() error {
	var ipvsSysctls = []struct {
		name  string
		value string
	}{
		{"net.ipv4.vs.conntrack", "0"},
		{"net.ipv4.vs.sloppy_tcp", "1"},
		{"net.ipv4.vs.conn_reuse_mode", "2"},
		{"net.ipv4.vs.schedule_icmp", "1"},
		{"net.ipv4.vs.expire_nodest_conn", "1"},
	}

	for _, ipvsSysctl := range ipvsSysctls {
		if _, err := util.Sysctl(ipvsSysctl.name, ipvsSysctl.value); err != nil {
			return err
		}
	}
	return nil
}
func EnsureIptables(ipt *iptables.IPTables, ipNets []*net.IPNet) error {
	// kube-proxy inserts a PREROUTING rule in the RAW table to redirect
	// externalIPs from LoadBalancers to the internal cluster IP instead as
	// an optimization. This bypasses it by creating a new RAW chain that disables
	// conntracking, preventing NAT from functioning

	// add new chain; ignore error since it only errors if chain already exists which we don't care
	_ = ipt.NewChain(IPTABLES_TABLE, IPTABLES_CHAIN)

	for _, cidr := range ipNets {
		// add CIDR rule to just stop tracking VIP CIDR's to prevent kube-proxy from doing its stupid thing
		acceptRule := []string{"-d", cidr.String(), "-j", "CT", "--notrack"}
		err := ipt.AppendUnique(
			IPTABLES_TABLE,
			IPTABLES_CHAIN,
			acceptRule...,
		)
		if err != nil {
			return err
		}
	}

	// if not match, return back to PREROUTING chain
	returnRule := []string{"-j", "RETURN"}
	err := ipt.AppendUnique(
		IPTABLES_TABLE,
		IPTABLES_CHAIN,
		returnRule...,
	)

	// insert rule to jump from PREROUTING to IPVS-CONTROLLER chain
	jumpRule := []string{"-j", IPTABLES_CHAIN}
	exists, err := ipt.Exists(IPTABLES_TABLE, IPTABLES_PREROUTING, jumpRule...)
	if err != nil {
		return err
	}
	if !exists {
		err := ipt.Insert(IPTABLES_TABLE, IPTABLES_PREROUTING, 1, jumpRule...)
		if err != nil {
			return err
		}
	}

	return nil
}

func LoadIpvsSchedulerKernelModulesIfExist() {
	for _, schedName := range SupportedIpvsSchedulers {
		kernelModuleName := fmt.Sprintf("ip_vs_%s", schedName)
		err := util.LoadKernelModule(kernelModuleName)
		if err != nil {
			glog.V(2).Infof(
				"error loading ipvs scheduler %s kernel module: %s",
				kernelModuleName,
				err.Error(),
			)
		}
	}
}
