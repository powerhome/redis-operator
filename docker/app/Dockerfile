# Pinned by digest so a build is reproducible and the image can be said to
# contain a known toolchain. The tags are kept alongside for readability: they
# are what the digests resolved to when they were recorded, not what resolves
# now.
# golang:1.25.14-alpine
FROM --platform=$BUILDPLATFORM golang@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
RUN apk --no-cache add \
  bash

WORKDIR /src
COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION

RUN TARGETOS=$TARGETOS TARGETARCH=$TARGETARCH VERSION=$VERSION ./scripts/build.sh

# alpine:3.24.1
FROM alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
RUN apk --no-cache add \
  ca-certificates
COPY --from=build /src/bin/redis-operator /usr/local/bin
RUN addgroup -g 1000 rf && \
  adduser -D -u 1000 -G rf rf && \
  chown rf:rf /usr/local/bin/redis-operator
USER rf

ENTRYPOINT ["/usr/local/bin/redis-operator"]
