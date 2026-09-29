// Special target: https://github.com/docker/metadata-action#bake-definition
target "docker-metadata-action" {}

// Default target if none specified
group "default" {
  targets = ["build-local"]
}

target "operator" {
  inherits = ["docker-metadata-action"]
  dockerfile = "docker/app/Dockerfile"
}

target "build-local" {
  inherits = ["operator"]
  output = ["type=docker"]
}

target "build" {
  inherits = ["operator"]
  // Published alongside the image so a consumer can answer "what is in this,
  // and where did it come from" from the registry, without pulling the image
  // and inferring its contents by scanning. The SBOM lists the Alpine packages
  // and the Go modules compiled into the binary; max-mode provenance records
  // the source revision and the resolved base image digests.
  //
  // Only on this target: the local and development targets are not published,
  // and attesting them would slow every local build for no reader.
  //
  // The scanner that produces the bill of materials is pinned like the base
  // images, so the build has no unpinned inputs. Unlike a base image, a stale
  // scanner degrades quietly rather than loudly: it does not know about package
  // ecosystems added after it was built, so it under-reports, and a document
  // that under-reports is worse than none because it still looks authoritative.
  // This pin needs bumping on a schedule, not when something breaks.
  attest = [
    "type=provenance,mode=max",
    "type=sbom,generator=docker/buildkit-syft-scanner:1.12.0@sha256:ae4f3b554449e7e25548e7d8ccc029d17357348e30c6e3df01b92bc93654d6a9",
  ]
  platforms = [
    "linux/amd64",
    "linux/arm/v6",
    "linux/arm/v7",
    "linux/arm64",
    "linux/386",
  ]
  // No cache backend, deliberately. Measured on a runner: `go mod download`
  // takes 2.8s, restoring that layer from the GitHub Actions cache takes 7.8s,
  // and exporting a mode=max cache of this build stage takes 49s once the cache
  // has content. The compile is 57s and misses on every commit that changes
  // source, which is all of them, so a cache can only ever save the few seconds
  // of dependency resolution and two apk calls. It costs an order of magnitude
  // more than that to carry.
}

// What a pull request builds. The binary is pure Go, compiled with CGO disabled
// and GOOS and GOARCH named, so the other four platforms drive the same source
// down the same path and catch nothing linux/amd64 misses. One platform also
// needs no emulation, because the runner is amd64 and the final stage's apk and
// adduser calls run natively.
//
// Publishing still covers every platform: master and release tags build the
// build target.
target "build-amd64" {
  inherits = ["build"]
  platforms = ["linux/amd64"]
}

variable UID { default = 1000 }
variable GID { default = 1000 }
target "dev" {
  dockerfile = "docker/development/Dockerfile"
  output = ["type=docker"]
  args = {
    uid: "${UID}",
    gid: "${GID}",
  }
}
