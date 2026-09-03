# Pinned by digest, like the release image, and kept on the same toolchain: the
# go directive in go.mod requires it, and these images set GOTOOLCHAIN=local so
# an older one cannot fetch a newer one to compensate.
FROM golang:1.25.14-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59

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

# Mock creator
ARG MOCKERY_VERSION="2.52.4"
RUN GOBIN=/go/bin go install github.com/vektra/mockery/v2@v${MOCKERY_VERSION}

# Create user
ARG uid=1000
ARG gid=1000
RUN addgroup -g $gid rf && \
    adduser -D -u $uid -G rf rf && \
    chown rf:rf -R /go

USER rf
WORKDIR /go/src/github.com/spotahome/redis-operator
