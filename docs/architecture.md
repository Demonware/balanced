# Architecture
## Components
### ipvs-controller
This controller manages the virtual servers and real servers deployed in LVS/IPVS and reflects the state of the Kubernetes Services and Endpoints. ipvs-controller should *only* be ran on nodes that will act as the load balancers. You should not be running ipvs-controller on worker nodes to avoid conflict. This can be done so by applying respective taints and tolerations. Please refer to the [example directory](../example/) for deployment examples.

### ipvs-host-controller
This controller is necessary for `ipvs-controller` to run as an unprivileged and pod-networking pod. It runs a subset of setup tasks that are required to be ran in privileged and host network mode. `ipvs-controller` must run in conjunction with a `pod-dep-init` initContainer to ensure that `ipvs-host-controller` is operational and running prior to
the start of an `ipvs-controller` Pod on the same node.

Please refer to the [pod networking example directory](../example/pod-network-mode)

### tunnel-controller
This controller provisions tunnel interfaces (and assign the service VIP address to it) on Pods that belong to one or more Loadbalancer(s). This allows for Direct-Server-Return (DSR) to function properly. Hence, tunnel-controller should run on all nodes in the cluster as a DaemonSet.

### vip-allocation-controller
This controller acts as the IPAM for Services (Type:Loadbalancers) deployed on a Kubernetes cluster. It will assign ExternalIPs to Services that meet requirements (Type: Loadbalancer and TargetPort match Port - see Known Limitations section). Only one vip-allocation-controller per IP pool should be run in the cluster. Although, at the moment, only one IP pool is supported - so run this as a Deployment with replicas set to 1. Additional IP pool support will be introduced in the future.
