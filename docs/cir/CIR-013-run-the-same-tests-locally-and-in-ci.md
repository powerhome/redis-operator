# CIR-013: Run the same tests locally and in CI

## Intent

Give each kind of test one target that runs the same way whoever calls it, so a
local pass predicts a CI pass, the way
[CIR-012](CIR-012-run-integration-tests-on-kind.md) already did for the
integration tests.

The unit and chart tests each had two targets instead: `make test-unit` and
`make test-helm` ran in the development container, `make test-unit-ci` and
`make test-helm-ci` ran the same commands on the host, and CI called the second
pair. Two things followed. Each kind of test had two environments that could
disagree, and the pair a contributor runs was the pair nothing gated. The
container pair also could not run anywhere without a terminal, because the
shared `docker run` asked for one: a CI step, a `pre-commit` hook, and any tool
driving make without a pseudo-terminal all got Docker's refusal to start the
container rather than a test result.

## Behavior

- GIVEN a machine with Go, of any patch version
- WHEN `make test-unit` runs
- THEN the unit tests run on the host against the toolchain the `go` directive
  in `go.mod` names, which Go fetches when the installed one is older

- GIVEN a machine with no helm installed
- WHEN `make test-helm` runs
- THEN the helm named in the Makefile is fetched into `bin/`, verified against
  its published checksum, and lints and templates the chart

- GIVEN `bin/` already holds that helm
- WHEN `make test-helm` runs again
- THEN nothing is fetched

- GIVEN a machine with a different helm installed
- WHEN `make test-helm` runs
- THEN the installed one is not used

- GIVEN a caller with no terminal, such as a CI step, a `pre-commit` hook, or a
  tool driving make
- WHEN `make test-unit` or `make test-helm` runs
- THEN it runs, because neither reaches for a container

- GIVEN the CI unit test and chart testing jobs
- WHEN they run
- THEN they call `make test-unit` and `make test-helm`, the targets a
  contributor calls, and install nothing a contributor does not

- GIVEN `make shell`
- WHEN it is run from a terminal
- THEN it still opens an interactive shell in the development container

- GIVEN the code generators and `make lint`
- WHEN they run
- THEN they still run in their pinned containers, now without asking for a
  terminal

- GIVEN the operator itself
- WHEN this change is applied
- THEN nothing about its behavior changes: it touches no operator source

## Constraints

- A fetched tool goes to `bin/<tool>-<version>-<os>-<arch>` and is verified
  against the checksum its release publishes, as `scripts/lint-workflows.sh`
  fetches actionlint and `scripts/integration-tests.sh` fetches kind. That
  script also carries the reason to prefer a release binary over a container
  image: an image comes from Docker Hub, which rate limits anonymous pulls by
  address, and CI runners share addresses with everyone else on the platform.
- `go.mod` names the Go toolchain and `GOTOOLCHAIN` defaults to `auto`, so a
  host resolves the same toolchain a container would pin.
- The development image stays. `make shell` is built on it, and `make
  generate-go` and `make generate-mocks` need the mockery it carries.

## Decisions

- **The unit tests run on the host, not in the development image.** The image
  pins `golang:1.25.14-alpine` by digest with `GOTOOLCHAIN=local`, which is how
  it guaranteed the Go version before `go.mod` carried a patch-level `go`
  directive. It does now, so the host guarantees the same thing by itself. Run
  both ways on this machine, the two agree exactly: the same 28 packages, the
  same status for each, 385 passing tests each.
- **The chart tests fetch a pinned helm rather than use whatever is installed.**
  Three helms are in play: 3.12.0 in the development image, 3.22.1 in CI, and
  4.3.1 on the machine this was written on. Rendering the chart with 3.12.0 and
  with 3.22.1 produces identical manifests, 216 lines either way, so nothing is
  broken. The pin is what makes that checkable rather than lucky:
  `helm lint` gains checks between releases, and the version that gates a merge
  should be the version a contributor runs.
- **The pin is 3.22.1, the version CI installs.** Moving it is a separate
  change with its own risk, and there is now one line to move rather than two
  files and an image to keep in step.
- **`test-unit-ci` and `test-helm-ci` are deleted, not kept as aliases.** Two
  names for one job is what let the two drift in the first place.
- **`DOCKER_INTERACTIVE` is removed rather than taught to detect a terminal.**
  Detection works, measured on GNU make 3.81 and 4.4.1, but it answers a
  question nothing needs to ask once the tests leave the container: a generator
  and a linter read no input, and `make shell` is the one target a terminal is
  the point of. It names `-it` itself.
- **`make lint` is left alone.** CI runs `golangci-lint-action` for the analysis
  cache it keeps, and reads the only pin of it from the Makefile, so the action
  and `make lint` cannot name different releases. The helm pin takes the same
  shape: one line in the Makefile, read by whoever needs it.

Measured on this change: `make test-helm` takes 1.23 seconds on the first run,
which includes fetching and verifying the helm release, and 0.12 seconds once
`bin/` holds it. The CI chart job loses its `azure/setup-helm` step and
pays that fetch instead.

## Date

2026-10-16
