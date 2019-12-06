package main

import (
	"fmt"
	"net"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/coreos/go-iptables/iptables"
	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"github.com/Demonware/balanced/pkg/pidresolver"
	"github.com/Demonware/balanced/pkg/util"
	api "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

const (
	IPTABLES_TABLE  = "filter"
	IPTABLES_CHAIN  = "TUNNEL-CONTROLLER"
	IPTABLES_OUTPUT = "OUTPUT"
)

type TunnelConfigurator interface {
	configure(*HostPod) error
}

type DummyTunnelConfigurator struct{}

func (dc *DummyTunnelConfigurator) configure(h *HostPod) error {
	glog.Infof("DummyTunnelConfigurator.configure: %s %+v %s", h.ID(), h.Addresses(), containerIDString(h.ContainerID()))
	return nil
}

type NamespaceTunnelConfigurator struct {
	eventRecorder record.EventRecorder
	pidResolver   pidresolver.PIDResolver
}

func (nc *NamespaceTunnelConfigurator) configure(h *HostPod) error {
	// TODO: mitigate around golang problem with network namespaces
	// https://www.weave.works/blog/linux-namespaces-and-go-don-t-mix
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if h.ContainerID() == nil {
		return fmt.Errorf("cannot find container id from HostPod: %s", h.ID())
	}
	var containerID = h.ContainerID()
	var addresses = h.Addresses()

	// used to reset and switch back to host network namespace - TODO: what happens if the GET retrieves the non-host network namespace
	// because of golang problem
	hostNetworkNamespaceHandle, err := netns.Get()
	if err != nil {
		return fmt.Errorf("Failed to get namespace: %s", err.Error())
	}
	defer hostNetworkNamespaceHandle.Close()
	defer func() {
		// always switch back to host namespace before returning from this function
		setNetworkNamespaceIgnoreError(hostNetworkNamespaceHandle)
		logCurrentNetworkNamespace()
	}()
	logCurrentNetworkNamespace()

	pid, err := nc.pidResolver.GetPID(*containerID)
	if err != nil {
		glog.Infof("Cannot resolve container %s, pod: %s, pid: %s", *containerID, h.ID(), err)
		return nil
	}
	glog.V(3).Infof("containerID %s resolved to pid: %d", *containerID, pid)

	endpointNamespaceHandle, err := netns.GetFromPid(pid)
	if err != nil {
		return fmt.Errorf("Failed to get endpoint namespace for %s (pid: %d): %s", *containerID, pid, err)
	}
	defer endpointNamespaceHandle.Close()
	err = netns.Set(endpointNamespaceHandle)
	if err != nil {
		return fmt.Errorf("Failed to enter to endpoint namespace: %s", err)
	}
	logCurrentNetworkNamespace()

	// get a ipip tunnel interface inside the endpoint container
	tunIf, err := netlink.LinkByName(util.KUBE_TUNNEL_IF)
	if err != nil {
		return fmt.Errorf("Failed to find iface %s, make sure ipip kernel module is loaded. %s",
			util.KUBE_TUNNEL_IF, err.Error())
	}
	// bring the tunnel interface up if not already up/unknown and actually have vips that need assigned
	if tunIf.Attrs().OperState != netlink.OperUnknown && tunIf.Attrs().OperState != netlink.OperUp && len(addresses) > 0 {
		err = netlink.LinkSetUp(tunIf)
		if err != nil {
			return fmt.Errorf("Failed to bring up ipip tunnel interface in endpoint namespace due to %s", err.Error())
		}
		if h.Pod() != nil {
			nc.eventRecorder.Eventf(
				h.Pod(), api.EventTypeNormal,
				"UppedIPIPTunnelInterface", "Set IPIP Tunnel %s to Up",
				util.KUBE_TUNNEL_IF)
		}
	}
	mainIfaceMTU := util.DEFAULT_KUBE_MAIN_IF_MTU
	// get main interface to get MTU from, ie. eth0
	mainIf, err := netlink.LinkByName(util.KUBE_MAIN_IF)
	if err != nil {
		glog.Infof("error finding main iface %s; cannot determine MTU - assuming 1500bytes", util.KUBE_MAIN_IF)
	} else if mainIf != nil {
		mainIfaceMTU = mainIf.Attrs().MTU
	}
	desiredTunlMTU := tunnelMTU(mainIfaceMTU)
	if tunIf.Attrs().MTU != desiredTunlMTU {
		err = netlink.LinkSetMTU(tunIf, desiredTunlMTU)
		if err != nil {
			glog.Infof("error setting %d MTU on iface: %s", desiredTunlMTU, err.Error())
			return err
		}
	}
	iptablesHandle, err := iptables.New()
	if err != nil {
		return fmt.Errorf("failed to create iptables handle: %s", err)
	}
	if err := UpdateIptables(iptablesHandle, addresses, desiredTunlMTU-util.TCP_MSS_DIFF); err != nil {
		return fmt.Errorf("updating MSS iptables rules: %s", err)
	}

	// assign VIPs to the KUBE_TUNNEL_IF interface
	for _, vip := range addresses {
		alreadyExists, err := util.AddIPtoInterface(vip, tunIf)
		if err != nil {
			glog.Infof("error adding vip %s to iface: %s", vip, err.Error())
			return err
		}
		if h.Pod() != nil && !alreadyExists {
			nc.eventRecorder.Eventf(
				h.Pod(), api.EventTypeNormal,
				"AddedIPIPTunnelIP", "Added Tunnel IP %s to IPIP Interface %s",
				vip, util.KUBE_TUNNEL_IF)
		}
		glog.V(4).Infof("Successfully assigned VIP %s to container %s", vip, *containerID)

	}

	// remove existing VIPs that are not in vips slice
	deletedIps, err := util.DeleteNonSuppliedIPsFromInterface(addresses, tunIf)
	if err != nil {
		return err
	}
	if h.Pod() != nil {
		for _, vip := range deletedIps {
			nc.eventRecorder.Eventf(
				h.Pod(), api.EventTypeNormal,
				"DeletedIPIPTunnelIP", "Removed Tunnel IP %s from IPIP Interface %s",
				vip, util.KUBE_TUNNEL_IF)
		}
	}
	return nil
}

func tunnelMTU(mainIfaceMTU int) int {
	return mainIfaceMTU - util.KUBE_TUNNEL_IF_OVERHEAD
}

func cleanupIptables(ipt *iptables.IPTables, addrs []net.IP, mss int) error {
	rules, err := ipt.List(IPTABLES_TABLE, IPTABLES_CHAIN)
	if err != nil {
		return err
	}

	var rulesToBeDeleted []string
	for _, rule := range rules {
		r := regexp.MustCompile(`-A.+-s (?P<Address>(?:[0-9]{1,3}\.){3}[0-9]{1,3})/32.+--set-mss (?P<MSS>\d+)`)
		match := r.FindStringSubmatch(rule)
		if len(match) < 3 {
			continue
		}
		ruleMss, err := strconv.Atoi(match[2])
		if err != nil {
			return err
		}
		// if rule's MSS does not match supplied MSS, just delete rule
		if ruleMss != mss {
			rulesToBeDeleted = append(rulesToBeDeleted, rule)
			continue
		}
		ipMatch := false
		for _, addr := range addrs {
			if addr.String() == match[1] {
				ipMatch = true
				break
			}
		}
		if !ipMatch {
			rulesToBeDeleted = append(rulesToBeDeleted, rule)
		}
	}
	for _, rule := range rulesToBeDeleted {
		ruleSlice := strings.Split(rule, " ")
		if err := ipt.Delete(IPTABLES_TABLE, IPTABLES_CHAIN, ruleSlice[2:]...); err != nil {
			return err
		}
	}
	return nil
}

func UpdateIptables(ipt *iptables.IPTables, addrs []net.IP, mss int) error {
	// need to add iptables rule to adjust ADVMSS on outgoing SYNACK to
	// properly account for the 20 byte overhead IPIP encap incurs

	// add new chain; ignore error since it only errors if chain already exists which we don't care
	_ = ipt.NewChain(IPTABLES_TABLE, IPTABLES_CHAIN)
	// remove old VIP entries from iptables
	if err := cleanupIptables(ipt, addrs, mss); err != nil {
		glog.Infof("error cleaning up iptables: %s", err.Error())
		return err
	}
	for _, addr := range addrs {
		mssRule := []string{
			"-s", addr.String(),
			"-p", "tcp",
			"-m", "tcp",
			"--tcp-flags", "SYN,RST,ACK", "SYN,ACK",
			"-j", "TCPMSS",
			"--set-mss", fmt.Sprintf("%d", mss),
		}
		exists, err := ipt.Exists(IPTABLES_TABLE, IPTABLES_CHAIN, mssRule...)
		if err != nil {
			return err
		} else if exists {
			continue
		}
		err = ipt.Insert(
			IPTABLES_TABLE,
			IPTABLES_CHAIN,
			1,
			mssRule...,
		)
		if err != nil {
			return err
		}
	}

	// if not match, return back to OUTPUT chain
	returnRule := []string{"-j", "RETURN"}
	err := ipt.AppendUnique(
		IPTABLES_TABLE,
		IPTABLES_CHAIN,
		returnRule...,
	)

	// insert rule to jump from OUTPUT to TUNNEL-CONTROLLER chain
	jumpRule := []string{"-j", IPTABLES_CHAIN}
	exists, err := ipt.Exists(IPTABLES_TABLE, IPTABLES_OUTPUT, jumpRule...)
	if err != nil {
		return err
	} else if !exists {
		err := ipt.Insert(IPTABLES_TABLE, IPTABLES_OUTPUT, 1, jumpRule...)
		if err != nil {
			return err
		}
	}
	return nil
}

func setNetworkNamespaceIgnoreError(handle netns.NsHandle) {
	err := netns.Set(handle)
	if err != nil {
		glog.V(4).Infof("Cannot set network namespace... %s", handle.String())
	}
}

func logCurrentNetworkNamespace() {
	// FIXME: its possible switch namespaces may never work safely in GO without hacks.
	//	 https://groups.google.com/forum/#!topic/golang-nuts/ss1gEOcehjk/discussion
	//	 https://www.weave.works/blog/linux-namespaces-and-go-don-t-mix
	// Dont know if same issue, but seen namespace issue, so adding
	// logs and boilerplate code and verbose logs for diagnosis
	activeNetworkNamespaceHandle, err := netns.Get()
	defer activeNetworkNamespaceHandle.Close()
	if err != nil {
		glog.V(4).Infof("Cannot retrieve current active network namespace: %s", err)
		return
	}
	glog.V(4).Infof("Current network namespace after revert namespace to host network namespace: %s", activeNetworkNamespaceHandle.String())
}
