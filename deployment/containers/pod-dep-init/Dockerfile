FROM alpine:3.10
ARG KUBECTL_VERSION=v1.13.10

RUN apk --no-cache add jq curl
RUN curl -L https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl -o /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl

COPY /entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]


