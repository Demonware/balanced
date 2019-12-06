## Host or Pod Networking Mode
`ipvs-controller` can run in both host-networking mode (default) or pod-networking mode. There are benefits and trade-offs for both.

### Advantages of Pod Networking Mode
- Security/Isolation: IPVS is ran within a normal pod (unprivileged) - this prevents any promiscously bound (0.0.0.0) services on the host from being exposed through the VIPs (eg. SSH)
- Stackable: Multiple `ipvs-controller` deployments can co-locate on the same node. Partitioning of VIPs may be useful for isolation and security purposes

### Disadvantages of Pod Networking Mode
- Extra Overhead: Requires a sidecar for the Pod to BGP peer with the host to share VIP route information - this introduces an additional hop to direct VIP traffic to the Pod, which may incur a cost on performance
- Lower Resilency (Caution Required): Any VIP traffic destined to the Pod will be handled by another IPVS L4D node when the Pod is deleted or rescheduled. Due to the stateless nature of Balanced, as long as there is enough remaining forwarding capacity, this behaviour should not cause any negative impact. Therefore, `ipvs-controller` updates need to be done carefully in a rolling-update fashion

Please refer to the [pod networking example directory](../example/pod-network-mode) for more information.
