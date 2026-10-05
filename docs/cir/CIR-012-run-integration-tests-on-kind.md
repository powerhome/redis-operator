# CIR-012: Run the integration tests on kind, locally and in CI

## Intent

Let a contributor run the integration tests on their own machine the way CI runs
them, with one command: `make test-integration`. CI calls the same target, so a
local pass predicts a CI pass.

## Behavior

- GIVEN a machine with Go and Docker
- WHEN `make test-integration` runs
- THEN a kind cluster is created, the tests run against it, and the cluster is
  deleted whether they pass or fail

- GIVEN a macOS host using Docker Desktop
- WHEN the tests dial a Redis or Sentinel pod IP
- THEN the connection succeeds, because the tests run inside the kind node
  container

- GIVEN a caller with an existing `~/.kube/config`
- WHEN `make test-integration` runs
- THEN the caller's current context is left as it was

- GIVEN the CI integration job
- WHEN it runs for one entry of its Kubernetes matrix
- THEN it runs `make test-integration` with that entry's kind node image

- GIVEN `make test`
- WHEN it runs
- THEN it runs the integration tests after the unit and chart tests

## Constraints

- The test in `test/integration/redisfailover` runs the operator inside the test
  process. It connects to Redis and Sentinel by pod IP and reads its kubeconfig
  from `~/.kube/config`. This change does not alter the test.
- A Docker Desktop host cannot route to addresses inside kind's Docker network.
- kind publishes node images for some Kubernetes patch releases only. The matrix
  can name only those, pinned by the digests in the
  [kind release notes](https://github.com/kubernetes-sigs/kind/releases).
- kind is fetched as a checksummed release binary into `bin/`, the way
  `scripts/lint-workflows.sh` fetches actionlint.

## Decisions

- **kind over minikube with the `none` driver.** The `none` driver installs
  Kubernetes onto the host itself. It needs root and Linux, so it cannot run on a
  contributor's Mac. In CI it needed a pinned minikube release and a workaround
  step for a missing CNI directory. kind runs on any host that has Docker.
- **Run the compiled tests inside the node container, not on the host.** The node
  can reach every pod IP without setup. Two alternatives were rejected:
  - Adding a host route to the pod network through the node's address works on a
    Linux runner, but needs root and does not work on Docker Desktop. Local and CI
    runs would then take different paths.
  - Running the tests in a separate container on kind's Docker network still needs
    a route to the pod network, which needs extra container privileges and a Go
    image from Docker Hub.
- **Build for the Docker host's architecture.** The binary runs in the node, so
  it is built for the architecture `docker version` reports. That can differ from
  the machine running `make`.
- **A single-node cluster.** It is what minikube provided, and the test does not
  depend on pods landing on different nodes.
- **No option to keep the cluster after a run.** It was left out to keep the
  script small. It can be added if inspecting a failed cluster locally proves
  necessary.

Measured on Docker Desktop on an Apple Silicon Mac: 297 seconds end to end, of
which the test took 249 and cluster creation 17.

## Date

2026-10-05
