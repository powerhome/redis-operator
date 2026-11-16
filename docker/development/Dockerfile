# Pinned by digest, like the release image, and kept on the same toolchain: the
# go directive in go.mod requires it, and these images set GOTOOLCHAIN=local so
# an older one cannot fetch a newer one to compensate.
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280

ARG TARGETOS
ARG TARGETARCH

RUN apk --no-cache add \
    bash \
    git \
    g++ \
    openssl

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
