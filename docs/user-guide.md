# User Guide

## Loadbalancer Creation
https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/

Example:
```yaml 
kind: Service
apiVersion: v1
metadata:
  name: my-loadbalancer
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
  type: LoadBalancer
```

### Service Annotations
#### IPVS scheduler (Advanced Users Only)
`balanced.demonware.net/ipvs-scheduler` sets the IPVS scheduler which will handle the scheduling and distribution of traffic to the IPVS Service

Useful Consistent Hashing Schedulers:
* `balanced.demonware.net/ipvs-scheduler=hrw`
* `balanced.demonware.net/ipvs-scheduler=mh`

```yaml 
kind: Service
apiVersion: v1
metadata:
  name: my-loadbalancer
  annotations:
    balanced.demonware.net/ipvs-scheduler: hrw
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
  type: LoadBalancer
```

#### IP Pool
`balanced.demonware.net/ip-pool` selects which IP Pool to assign a VIP address from. Leave blank for default IP Pool.

IP Pool can be a CIDR or a label:
* `balanced.demonware.net/ip-pool=192.168.1.0/24`
* `balanced.demonware.net/ip-pool=home-network`

```yaml 
kind: Service
apiVersion: v1
metadata:
  name: my-loadbalancer
  annotations:
    balanced.demonware.net/ip-pool: home-network
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
  type: LoadBalancer
```

#### IP Request
`balanced.demonware.net/ip-request` specifies a preference of IP addresses to be allocated for the Loadbalancer service. It can be a single IP address or a comma-delimited list in the order of preferences from first to last, ie. The **first** IP address that is available from the list will be allocated to the Service.

IP addresses specified in the request are allocated based off of availability. This annotation can be used in junction with IP Pool if the requested IP address is not within the default IP Pool subnet.

**It is important to note** that if no IP addresses are available from the requested list, an IP address *will* not be allocated to the Service. You must make amends to the list or remove the annotation completely, in order to move forward.

Example of a requested single IP address:
```yaml 
kind: Service
apiVersion: v1
metadata:
  name: my-loadbalancer
  annotations:
    balanced.demonware.net/ip-pool: home-network
    balanced.demonware.net/ip-request: "192.168.1.100"
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
  type: LoadBalancer
```

Example of multiple IP address candidates:
```yaml 
kind: Service
apiVersion: v1
metadata:
  name: my-loadbalancer
  annotations:
    balanced.demonware.net/ip-pool: home-network
    balanced.demonware.net/ip-request: "192.168.1.100, 192.168.1.101, 192.168.1.102"
spec:
  selector:
    app: MyApp
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
  type: LoadBalancer
```


### Caveats
1. `externalTrafficPolicy` and `healthCheckNodePort` fields on the Service Spec are ignored
2. Loadbalancer Port must be the same as Target Port (ie. Port == TargetPort in the Spec). Otherwise, the Endpoint/Pod will not be added to the loadbalancer
