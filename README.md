<img src="images/logo.png" title="BalanceD" width="25%" />

**BalanceD** is a Layer-4 Linux Virtual Server (LVS) based load balancing platform for Kubernetes.

It is capable of providing basic load balancing for any Kubernetes cluster and is reliable and performant. This is achieved by utilizing LVS (also known as IPVS - IP Virtual Server), along with any Consistent-Hashing algorithm, to implement a stateless Direct-Server-Return (DSR) Layer-4 load balancer. The stateless nature means it can be horizontally scaled with the help of AnyCast and ECMP.

Above all, it is designed with modularity in mind which makes BalanceD very simple to operate and maintain.

BalanceD is meant to be placed alongside an existing Kubernetes networking solution, and is *NOT* a turnkey solution for Kubernetes networking such as [kube-router](https://github.com/cloudnativelabs/kube-router), which was an inspiration for this project.

## Status
BalanceD is being used in several production clusters to power the most demanding online game services at [Demonware](http://demonware.net)! Core functionality is considered stable.

## Requirements
BalanceD requires two LVS/IPVS nodes at minimum for redundancy purposes - these can either be physical machines or virtual instances. Each node must be part of the Kubernetes cluster and can route to all Pods through their respective Pod IPs.
Each node must be configured to act as BGP peers to advertise the VIP addresses to the rest of the network. For flexibility, BalanceD does not bundle a BGP speaker allowing for a more seamless integration with the network. A BIRD configuration is available [here](example/bird.conf) as a reference.

### Kubernetes
BalanceD has been tested on versions Kubernetes >= 1.9. If kube-proxy is running in IPVS mode, the `ipvs-controller` of BalanceD must run in Pod Networking Mode to avoid conflict with IPVS.

### Operating System
BalanceD was tested to operate on Linux kernels >= 4.4. However, it should work in theory with any modern kernels with decent LVS/IPVS support.

### Supported CNI Plugins
This has been tested to work with:
- [Bridge CNI Plugin](https://github.com/containernetworking/plugins/blob/master/plugins/main/bridge/README.md)

## Getting Started

- [End User Guide](./docs/user-guide.md)
- [Architecture](./docs/architecture.md)
- [Host vs Pod Networking Mode](./docs/host-vs-pod-networking.md)
- [Limitations](./docs/limitations.md)

## Build
### Docker
`make images` to build all controllers.

### Golang
*Go version 1.12 or above is required to build BalanceD*  
`make build` to build all controllers.

## Acknowledgement

BalanceD is built upon following open-source libraries:

- go-iptables: https://github.com/coreos/go-iptables
- netlink: https://github.com/vishvananda/netlink
- libnetwork: https://github.com/docker/libnetwork

and many more!

BalanceD has also been inspired by:
- kube-router: https://github.com/cloudnativelabs/kube-router


## Support and Feedback

Feel free to leave feedback or raise any questions by opening an issue [here](https://github.com/Demonware/balanced/issues).

## Contributing

We encourage all kinds of contributions! Feel free to submit a PR!
