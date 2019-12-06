# vip-dep-init
A container used as an initContainer to wait and block until the balanced ipvs-tunel-controller initializes the tunnel interface and bind corresponding service VIP(s) to it.

This can be used for UDP applications to explicitly bind to a VIP such that responses can be directed back to the client properly.

## Usage
### Environment Variables

Use the following three required variables to select the Pod to block on
`TUNL_INTERFACE`
