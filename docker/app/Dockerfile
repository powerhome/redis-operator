# Pinned by digest so a build is reproducible and the image can be said to
# contain a known toolchain. The tag names what the digest was when it was
# recorded; the digest is what actually resolves, and it wins if the two ever
# disagree.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
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

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
RUN apk --no-cache add \
  ca-certificates
COPY --from=build /src/bin/redis-operator /usr/local/bin
RUN addgroup -g 1000 rf && \
  adduser -D -u 1000 -G rf rf && \
  chown rf:rf /usr/local/bin/redis-operator
USER rf

ENTRYPOINT ["/usr/local/bin/redis-operator"]
