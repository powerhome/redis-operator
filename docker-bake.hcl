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
  attest = [
    "type=provenance,mode=max",
    "type=sbom",
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
