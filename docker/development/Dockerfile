FROM golang:1.20-alpine

ARG TARGETOS
ARG TARGETARCH

RUN apk --no-cache add \
    bash \
    git \
    g++ \
    openssl

# Helm for running helm template tests locally
ENV HELM_VERSION="v3.12.0"
RUN wget -c https://get.helm.sh/helm-${HELM_VERSION}-${TARGETOS}-${TARGETARCH}.tar.gz -O - | tar zxv -C /tmp && \
    mv /tmp/${TARGETOS}-${TARGETARCH}/helm /go/bin/helm

# Code generator stuff
# Check: https://github.com/kubernetes/kubernetes/pull/57656
ENV CODEGEN_VERSION="1.11.9"
RUN wget http://github.com/kubernetes/code-generator/archive/kubernetes-${CODEGEN_VERSION}.tar.gz && \
    mkdir -p /go/src/k8s.io/code-generator/ && \
    tar zxvf kubernetes-${CODEGEN_VERSION}.tar.gz --strip 1 -C /go/src/k8s.io/code-generator/ && \
    mkdir -p /go/src/k8s.io/kubernetes/hack/boilerplate/ && \
    touch /go/src/k8s.io/kubernetes/hack/boilerplate/boilerplate.go.txt

# Mock creator
ARG MOCKERY_VERSION="2.32.0"
RUN wget -c https://github.com/vektra/mockery/releases/download/v${MOCKERY_VERSION}/mockery_${MOCKERY_VERSION}_$(uname -o)_${TARGETARCH}.tar.gz -O - | tar -xz -C /go/bin/

# Create user
ARG uid=1000
ARG gid=1000
RUN addgroup -g $gid rf && \
    adduser -D -u $uid -G rf rf && \
    chown rf:rf -R /go

USER rf
WORKDIR /go/src/github.com/spotahome/redis-operator
