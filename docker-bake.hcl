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
