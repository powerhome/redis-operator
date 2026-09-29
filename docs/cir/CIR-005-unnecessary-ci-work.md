# CIR-005: Remove unnecessary work from CI

## Intent

Make a CI run finish sooner, and give a transient network failure fewer chances to
fail one, by removing four pieces of work CI does that it does not need to do.

The exposure is not hypothetical. A build on
[#132](https://github.com/powerhome/redis-operator/pull/132) failed on the module
proxy rather than on anything in the change, and passed on a re-run:

```text
golang.org/x/text@v0.41.0: read "https://proxy.golang.org/golang.org/x/text/@v/v0.41.0.zip":
stream error: stream ID 167; INTERNAL_ERROR; received from peer
```

A re-run is the only remedy for that, so what wants fixing is the exposure rather
than the one failure.

Measured on the master run of commit `8e23d55d` (run 36205162337) and on the
comparable pull request run 36203557493, 16 minutes of wall clock:

- `Golang Check`, 63 to 91 seconds, fetched and recompiled every dependency,
  because module caching is switched off in that job alone.
- `dockerhub-image`, 348 seconds, built five platforms, compiling the same pure-Go
  source five times under emulation and asking the module proxy once per platform.
- `Workflow lint` pulled `rhysd/actionlint` from Docker Hub anonymously, against
  limits shared with every other customer on the runner's address.
- Each integration leg installed conntrack twice, once itself and once inside
  `medyagh/setup-minikube`, which installs it for any `none` driver.

Two things that look like they belong on that list do not. The Go toolchain is
never downloaded, and the image build's module fetch is not worth caching; see
Decisions for both.

## Behavior

- GIVEN the lint job resolving dependencies
- WHEN it runs on a commit whose `go.sum` is unchanged
- THEN they come from the Go module cache rather than the module proxy

- GIVEN a source-only change and a previous local build
- WHEN `make image-local` runs
- THEN the dependency layer is reused and no module is fetched

- GIVEN any build in CI
- WHEN it runs
- THEN it exports no build cache and imports none, so the only state it carries
  between runs is what the registry already holds

- GIVEN a pull request
- WHEN the operator image builds
- THEN a `linux/amd64` image publishes under the pull request tag, and no other
  platform is built

- GIVEN a push to master or a release tag
- WHEN the operator image builds
- THEN all five published platforms are built, with provenance and a bill of
  materials

- GIVEN the workflows are linted
- WHEN actionlint is needed
- THEN it is fetched from its GitHub release, verified against that release's
  checksum file, kept in `bin/` under its version and platform, and Docker Hub is
  not contacted

- GIVEN the integration job's none driver
- WHEN its dependencies are installed
- THEN `medyagh/setup-minikube` installs them once, and the job installs none of
  them separately

- GIVEN a local build (`make image-local`, the `build-local` bake target)
- WHEN it runs
- THEN it builds the host platform alone with no attestations, and needs no
  container registry beyond the pinned base images

- GIVEN the chart jobs, the CRD drift gate and the chart version gate
- WHEN any of the above changes
- THEN their behavior SHOULD NOT change

## Constraints

- **Runners are ephemeral, so a Dockerfile layer is not by itself a CI
  optimization.** A dependency layer survives between local rebuilds and is
  reconstructed from nothing on every CI run. Carrying it across runs needs a
  cache backend, which has its own cost, so the two have to be judged on measured
  numbers rather than on the layer's obvious appeal.
- **Fork pull requests get no secrets.** This repository is a public fork, so any
  fix that depends on a registry credential works for branch pull requests and
  not for fork pull requests. The publish steps already have that property; a
  lint job should not acquire it.
- **A change made to reduce transient failures must not add a new way for one to
  fail a run.** Anything optional on the path of a publish has to be able to fail
  without taking the publish with it, or it does not belong there.

## Decisions

- **Pull requests build `linux/amd64` alone.** `scripts/build.sh` runs
  `CGO_ENABLED=0 go build` with `GOOS` and `GOARCH` set. There is no C toolchain
  per platform and no build tag in the tree, so the other four platforms drive the
  same source down the same path. The accepted cost: a genuinely
  platform-specific break lands on master rather than being caught on the pull
  request. Master builds all five platforms on every push, so such a break
  surfaces before any release tag is cut, and no release publishes without a
  five-platform build having passed.

  Related, and not fixed here: `scripts/build.sh` reads `TARGETARCH` but not
  `TARGETVARIANT`, and `linux/arm/v6` and `linux/arm/v7` both set
  `TARGETARCH=arm`. `GOARM` is never set, so those two targets compile
  identically today and the `arm/v6` image carries whatever Go's default is. Two
  of the five platforms are therefore not distinct builds regardless of this
  decision.

- **Rejected: a GitHub Actions build cache for the image.** `cache-from` and
  `cache-to` with `type=gha,mode=max` was implemented and measured over two runs
  before being removed. On a runner: `go mod download` takes 2.8 seconds and
  restoring that layer from the cache takes 7.8, so the import loses on the very
  layer the cache was added for. The compile is 57 seconds and misses on every
  commit that changes source, so it is never cached in practice. The export costs
  96 seconds cold and 49 warm. Against a total available saving of roughly five
  seconds, the backend made `dockerhub-image` 203 seconds cold and 165 warm where
  no cache at all measures 101 to 123 across two runs.

  The error behind adding it is worth naming: 216 `go: downloading` lines in a run
  log were read as a cost. A line count is not a duration, and resolving all 43
  modules against a nearby proxy is under three seconds. The 348 seconds were the
  five platforms.

  The Dockerfile's dependency layer stays. It costs nothing, it is the right shape
  for a Go Dockerfile, and it serves local rebuilds. `docker-bake.hcl` carries
  these numbers where someone reaching for a cache backend will find them.

- **Rejected: authenticating the actionlint pull to Docker Hub.** The credentials
  exist as repository secrets, but a fork pull request receives none, so the lint
  job would fail for exactly the contributor least able to diagnose it.

- **Rejected: `go run github.com/rhysd/actionlint/cmd/actionlint@vX.Y.Z`.** It
  removes Docker Hub and reuses the module proxy the build already depends on,
  but it compiles actionlint on every run and requires `setup-go` in a job that
  otherwise needs no Go. The release binary is a two second fetch.

- **Accepted: verify actionlint against the release's own checksum file, not a
  hash pinned in this repository.** A pinned per-platform hash is stronger
  against substitution, and it is also a hand-maintained value per platform per
  version bump, for a lint tool. The checksum file catches the truncated or
  corrupted download, which is how this fetch fails in practice. Base images and
  the bill-of-materials scanner stay pinned by digest; they end up inside a
  published artifact, and actionlint does not.

- **Accepted: the lint job caches Go modules through `setup-go`.**
  `golangci-lint-action` caches `~/.cache/golangci-lint` and nothing else, which
  is 927 KB of analysis results, and its documentation says it relies on
  `setup-go` for the module cache. `cache: true` is written explicitly so the
  reason is visible at the call site.

- **minikube's action moves to v0.0.21; minikube itself stays at 1.38.1.** The
  action carries `@actions/cache` 4.0.3 in place of 3.2.4, which is what the
  retired v1 cache service requires, so the three failed restores per leg stop.
  Those restores are the iso, kic and preloaded-tarball caches, none of which a
  `none` driver uses, so the warnings were noise rather than a cost: crictl, the
  CNI plugins and cri-dockerd are fetched by unconditional `downloadTool` calls
  that the action does not cache, and still are.
  It also moves to `node24` and bumps crictl from v1.26.1 to v1.34.0, a version
  skew worth closing on its own against Kubernetes 1.34 through 1.36. minikube
  1.39.0 was not taken with it: it makes containerd the default container
  runtime, and the none driver here is set up for dockerd, so that move is
  either an explicit `container-runtime: docker` pin or a conversion to
  containerd, and it belongs in its own change.

- **Rejected: treating the Go toolchain's tool cache hit as something to
  maintain.** `go-version-file: go.mod` asks for an exact patch, and the runner
  images happen to preinstall it, so no job downloads Go. Holding that true means
  tracking runner image release notes forever, and the penalty for losing it is
  one 80 MB fetch per job. The fact is recorded as a comment on the `go`
  directive and nothing enforces it.

## Verification

Run locally against this branch:

- `make lint-workflows` on macOS and, in an `ubuntu:22.04` container, on Linux.
  Both fetch, verify the checksum and lint clean. The second run on each skips
  the fetch. `bin/` names the binary by version and platform, so a repository
  shared between a host and a container does not reuse the wrong one.
- `docker buildx bake build-local` builds and the `go mod download` layer
  succeeds. Appending to `cmd/redisoperator/main.go` and rebuilding leaves that
  layer `CACHED`, which is the reuse the layer exists for.
- `docker buildx bake --print` for all three targets: `build-amd64` is one
  platform, `build` is five, neither declares a cache backend, and `build-local`
  carries no attestations. Printed against a stand-in metadata bake file,
  `build-amd64` inherits the tags and labels, so a pull request still publishes
  its tag.
- `scripts/build.sh` makes no assumption about the state of the module directory
  before the split: no `-mod=` flag, no vendor handling, no reference to it at
  all. A `go build` that expected to resolve dependencies itself could behave
  differently against a cache the new layer has already populated.
- `make test-unit-ci` and `golangci-lint run` pass.
- `actionlint` passes on the edited workflow.

Measured on runners. All four are pull request runs on the same three-leg matrix:
36203557493 is the baseline, 36478251386 and 36494386363 carried the build cache
backend cold and warm, and 36495911648 is this branch as it stands.

| Job | Baseline | Cold cache | Warm cache | No cache |
|---|---|---|---|---|
| Golang Check | 63s | 39s | 22s | 26s |
| Workflow lint | 9s | 6s | 7s | 4s |
| Integration, 1.34.4 leg, setup | 118s | 103s | 108s | 102s |
| Integration, 1.34.4 leg, tests | 427s | 428s | 425s | 426s |
| dockerhub-image | 348s | 203s | 165s | 101s |
| Wall clock | 16m02s | 13m20s | 12m08s | 11m04s |

The two cache columns are why the backend is gone: it made the image build slower
than no cache at all, in both directions. Run 36495911648 logs no cache export
step.

Every figure above is one sample from a shared runner. Run 36497116665 built the
same no-cache configuration and measured `dockerhub-image` at 123s and wall clock
at 11m35s, against 101s and 11m04s the run before; the compile alone moved between
51 and 58 seconds across the two. So read the conclusions as ratios large enough
to survive that spread, not as the individual seconds: a 49 second export against
under 3 seconds of dependency resolution, and one platform against five.

- The lint job restores the 349 MB module cache under the key the unit test job
  already writes. That is the whole of its improvement, and the second run is
  faster still because golangci-lint's own analysis cache is then warm on the
  branch too.
- `setup-minikube` v0.0.21 logs no failed cache restores, down from three a leg.
- The image build runs one platform (`RUN TARGETOS=linux TARGETARCH=amd64`) and
  skips emulation setup.
- actionlint comes from its GitHub release, and Docker Hub is not contacted.
- The test phase is 427 seconds before and after, which is the intent: none of
  this was supposed to change what the tests do. Leg totals therefore move only
  with test-time noise, about 20 seconds either way.

## Date

2026-09-28
