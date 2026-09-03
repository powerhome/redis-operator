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
