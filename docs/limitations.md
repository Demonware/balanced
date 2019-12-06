## Known Limitations
### Loadbalancer Port must be the same as TargetPort
#### Explanation
This limitation is due to the Direct-Server-Return model. The Layer-4 Director (the IPVS node) will encapsulate incoming packets using IPIP without rewriting the Destination Port of the original packet. This means that if the Loadbalancer port is different, the underlying service would not receive the packets destined to it once the tunnel interface within the Pod decapsulates the IPIP packet.
#### Symptom
If Loadbalancer port does not match a target port, a service VIP address will not be allocated by the vip-allocation-controller for that specific Kubernetes Service (type: Loadbalancer). 
#### Resolution
Make sure the controller port is identical to the loadbalancer port when creating a Kubernetes Service.

### TCP Application should be listening to all IP addresses (0.0.0.0) or VIP address on the given TargetPort
#### Explanation
This limitation is due to the Direct-Server-Return model. If the underlying application is only binding to eth0 or listening to a specific IP address, it would not receive the packets destined to it as the destination IP on those packets will be set as the VIP address instead of eth0's IP address.

The application may also specifically bind to a VIP address. The Pod that hosts the container will require a `vip-dep-init` initContainer to wait for tunnel interface initialization prior to the main container from being started. An example can be found [here](../example/udp-app-example/01-deployment.yaml#L12-L22)

#### Symptom
Connection will be refused by the Pod and the underlying application.
#### Resolution
Ensure that either: a) the application is bound to the specified TargetPort on all interfaces and addresses (0.0.0.0:port or *:port) or b) the application is bound to the specific VIP address and TargetPort ([example here](../example/udp-app-example/01-deployment.yaml#L27-L29))

### UDP Application should be listening to the VIP address on the given TargetPort
When an application opens a UDP socket that is bound to 0.0.0.0 (INADDR_ANY), it allows for listening of packets on all interfaces; the application, in theory, should be able to respond to a request using the same IP address a packet is received on. However, that is not the case when an application receives a UDP packet through balanced/IPVS due to packet encapsulation. There are mechanisms to [fix this which require major application code changes](https://stackoverflow.com/questions/3062205/setting-the-source-ip-for-a-udp-socket) to change how a UDP response is formed, which is not always feasible or possible. The only requirement is that the application must allow for the configuration of an IP address to bind UDP socket(s) to.

#### Symptom
UDP responses/replies sent from the Application will not be received by the client.
#### Resolution
Ensure that the application is bound to the specific VIP address and TargetPort ([example here](../example/udp-app-example/01-deployment.yaml#L27-L29))

### Scale up of Backend/Pods may lead to connections being terminated and balanced
#### Explanation
With the introduction of additional backends with the highest-random-weight consistent-hashing algorithm, it is normal for a portion of existing connections to existing backends to be moved over to the newly introduced backend. This can be better visualized by a ring representation: https://akshatm.svbtle.com/consistent-hash-rings-theory-and-implementation

However, we make use of a feature available in LVS/IPVS (conn_reuse_mode=2) to mitigate this behaviour somewhat. Only if/when there's a scale event on the IPVS Layer-4 director layer along with existing bucket changes in the backend, will connections be re-balanced. This drops the probablity of persistent connections being severed when scaling up a service.
