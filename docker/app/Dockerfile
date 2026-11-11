# Pinned by digest so a build is reproducible and the image can be said to
# contain a known toolchain. The tag names what the digest was when it was
# recorded; the digest is what actually resolves, and it wins if the two ever
# disagree.
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS build
RUN apk --no-cache add \
  bash

WORKDIR /src

# Dependencies resolve in a layer of their own, before the source arrives, so the
# layer is keyed on go.mod and go.sum alone and a source-only change reuses it
# instead of fetching every module again.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION

RUN TARGETOS=$TARGETOS TARGETARCH=$TARGETARCH VERSION=$VERSION ./scripts/build.sh

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
RUN apk --no-cache add \
  ca-certificates
COPY --from=build /src/bin/redis-operator /usr/local/bin
RUN addgroup -g 1000 rf && \
  adduser -D -u 1000 -G rf rf && \
  chown rf:rf /usr/local/bin/redis-operator
USER rf

ENTRYPOINT ["/usr/local/bin/redis-operator"]
