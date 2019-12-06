# pod-dep-init
A container used as an initContainer to wait and block for another pod to be running on the same host.

This is used for ipvs-controller (balanced) in pod network mode

## Usage
### Environment Variables

Use the following three required variables to select the Pod to block on
`NODE_NAME`

`NAMESPACE`

`LABEL_SELECTOR`
