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
    openssl \
    tar

# Mock creator. The Makefile holds the only pin and passes it in, so the two
# cannot disagree about which release this image carries. There is no default:
# a build that names no release fails here rather than installing one nobody
# chose. The Makefile says what constrains the choice. See CIR-015.
ARG MOCKERY_VERSION
RUN GOBIN=/go/bin go install github.com/vektra/mockery/v2@v${MOCKERY_VERSION}

# The Kubernetes client, lister and informer code, and the CRD manifest. The
# wrapper runs the generators out of a checkout of code-generator, compiling
# them as it goes, so this image carries that source and the Go that compiles
# it. A generator compiled by a Go older than the toolchain cannot type check
# what that toolchain compiles. The Makefile holds the only pin of each of the
# three, and none has a default here, for the reason the mockery pin gives.
# See CIR-016.
ARG KUBE_CODE_GENERATOR_VERSION
RUN GOBIN=/go/bin go install \
    github.com/slok/kube-code-generator/cmd/kube-code-generator@v${KUBE_CODE_GENERATOR_VERSION}

ARG CONTROLLER_GEN_VERSION
RUN GOBIN=/go/bin go install \
    sigs.k8s.io/controller-tools/cmd/controller-gen@v${CONTROLLER_GEN_VERSION}

ARG CODEGEN_VERSION
ENV KUBE_CODE_GENERATOR_CODEGEN_PATH=/opt/k8s-code-generator
RUN mkdir -p ${KUBE_CODE_GENERATOR_CODEGEN_PATH} && \
    wget -qO- "https://github.com/kubernetes/code-generator/archive/kubernetes-${CODEGEN_VERSION}.tar.gz" \
      | tar zx --strip-components=1 -C ${KUBE_CODE_GENERATOR_CODEGEN_PATH}

# Create user
ARG uid=1000
ARG gid=1000
RUN addgroup -g $gid rf && \
    adduser -D -u $uid -G rf rf && \
    chown rf:rf -R /go ${KUBE_CODE_GENERATOR_CODEGEN_PATH}

USER rf
RUN cd ${KUBE_CODE_GENERATOR_CODEGEN_PATH} && go mod download
WORKDIR /go/src/github.com/spotahome/redis-operator
