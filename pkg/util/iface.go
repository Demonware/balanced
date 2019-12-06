package util

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/golang/glog"
	"github.com/vishvananda/netlink"
)

const (
	KUBE_TUNNEL_IF    = "tunl0"
	KUBE_DUMMY_IF     = "dummy-vip-if"
	KUBE_MAIN_IF      = "eth0"
	IFACE_NOT_FOUND   = "Link not found"
	IFACE_HAS_ADDR    = "file exists"
	IFACE_HAS_NO_ADDR = "cannot assign requested address"

	DEFAULT_KUBE_MAIN_IF_MTU = 1500
	KUBE_TUNNEL_IF_OVERHEAD  = 20
	TCP_MSS_DIFF             = 40
)

func GetDummyVipInterface() (netlink.Link, error) {
	var dummyVipInterface netlink.Link
	dummyVipInterface, err := netlink.LinkByName(KUBE_DUMMY_IF)
	if err != nil {
		if err.Error() == IFACE_NOT_FOUND {
			glog.V(1).Infof("Could not find dummy interface: " + KUBE_DUMMY_IF + " to assign VIP's, creating one")
			err = netlink.LinkAdd(&netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: KUBE_DUMMY_IF}})
			if err != nil {
				return nil, errors.New("Failed to add dummy interface:  " + err.Error())
			}
			dummyVipInterface, err = netlink.LinkByName(KUBE_DUMMY_IF)
		} else {
			return nil, err
		}
	}

	err = netlink.LinkSetUp(dummyVipInterface) // make sure link is always up
	if err != nil {
		return nil, errors.New("Failed to bring dummy interface up: " + err.Error())
	}
	return dummyVipInterface, nil
}

func AddIPtoInterface(ip net.IP, iface netlink.Link) (bool, error) {
	netAddr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Scope: syscall.RT_SCOPE_LINK,
	}
	err := netlink.AddrAdd(iface, netAddr)
	// only treat as error if addr is not on iface; already added errors shouldn't be treated as such
	if err != nil {
		if err.Error() != IFACE_HAS_ADDR {
			return false, fmt.Errorf(
				"Failed to assign IP %s to iface %s: %s",
				ip,
				iface,
				err.Error())
		} else {
			// return true if IP already exists
			return true, nil
		}
	}
	return false, nil
}

func DeleteIPfromInterface(ip net.IP, iface netlink.Link) error {
	netAddr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   ip,
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Scope: syscall.RT_SCOPE_LINK,
	}
	err := netlink.AddrDel(iface, netAddr)
	// only treat as error if addr is not about ip not on iface; non-existant address shouldn't be treated as such
	if err != nil && err.Error() != IFACE_HAS_NO_ADDR {
		return fmt.Errorf(
			"Failed to remove IP %s to iface %s: %s",
			ip,
			iface,
			err.Error())
	}
	return nil
}

func GetNetAddrsfromInterface(iface netlink.Link) ([]netlink.Addr, error) {
	return netlink.AddrList(iface, netlink.FAMILY_V4)
}

func DeleteNonSuppliedIPsFromInterface(addresses []net.IP, iface netlink.Link) ([]string, error) {
	var suppliedIPBoolMap = make(map[string]bool)
	for _, addr := range addresses {
		suppliedIPBoolMap[addr.String()] = true
	}
	var deletedIps []string

	netAddrs, err := GetNetAddrsfromInterface(iface)
	if err != nil {
		return nil, err
	}
	for _, netAddr := range netAddrs {
		ip := netAddr.IPNet.IP.String()
		if _, exists := suppliedIPBoolMap[ip]; !exists {
			glog.V(3).Infof("Deleting IPv4 %s from iface %s because not in supplied list", ip, iface.Attrs().Name)
			err := netlink.AddrDel(iface, &netAddr)
			if err != nil {
				return nil, err
			}
			deletedIps = append(deletedIps, ip)
		}
	}
	return deletedIps, nil
}

func convStringSliceToBoolMap(s []string) map[string]bool {
	boolMap := make(map[string]bool)
	for _, v := range s {
		boolMap[v] = true
	}
	return boolMap
}
