from golang:1.12 as cache
env GO111MODULE=on
workdir github.com/Demonware/balanced
copy go.mod .
copy go.sum .
run go mod download

from cache as builder
arg CMD
copy . .
run make target/cmd/${CMD} && cp target/cmd/${CMD} /balanced-app

FROM alpine:3.7
copy --from=builder /balanced-app /balanced-app
RUN apk add --no-cache \
      iptables \
      ipset \
      iproute2 \
      ipvsadm
entrypoint ["/balanced-app"]
