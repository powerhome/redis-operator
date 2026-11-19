# CIR-016: Generate Kubernetes code against the pinned toolchain

## Intent

Keep `make generate-client` and `make generate-crd` able to run. Changing an
API type should be an ordinary change, not one that has to work around its own
tooling.

Both targets ran a published generator image. That image carries its own Go, and
it compiles the generators out of a checkout of `code-generator` as it runs
them. A generator compiled by a Go older than the toolchain cannot type check
what that toolchain compiles.

Go 1.27 added methods with type parameters, and its standard library uses one:

```go
func (r *Rand) N[Int intType](n Int) Int
```

A generator built by Go 1.26 or older rejects that source outright, so
generation stopped on `error(s) in "math/rand/v2": method must have no type
parameters` and wrote nothing. That is where the repository stood: the `go`
directive moved to 1.27.4 while both targets ran an image built on Go 1.24.

This is the fault [CIR-015](CIR-015-generate-mocks-against-the-pinned-toolchain.md)
recorded for mockery, reaching a second generator.

## Behavior

- GIVEN a checkout on the toolchain `go.mod` names
- WHEN `make generate-client` runs
- THEN `client/k8s` is rewritten, and the only difference from the committed
  copies is whatever the API types changed

- GIVEN a development image built without one of the three releases named
- WHEN the image builds
- THEN the build fails at that step, rather than installing one nobody chose

- GIVEN a new Go toolchain release
- WHEN Renovate proposes it
- THEN the three travel in the same pull request, the way mockery does

- GIVEN a machine that is not amd64
- WHEN either target runs
- THEN it runs natively, because no step needs an emulated image

- GIVEN an unchanged checkout
- WHEN `make generate-crd` runs
- THEN it rewrites the CRD, which differs from the committed copy by about
  1,200 lines

## Constraints

- **The Makefile holds one pin per tool.** golangci-lint, helm, kind and
  mockery each live there with a `# renovate:` comment above them, read by
  `customManagers:makefileVersions`.
  [CIR-013](CIR-013-run-the-same-tests-locally-and-in-ci.md) is why.
- **The wrapper needs the generators as source, not as binaries.**
  It requires a `--codegen-path`, and runs what it finds there, compiling as it
  goes. So the image carries a `code-generator` tree and the Go that compiles
  it.
- **No published tag of the generator image ships a Go new enough.** The newest
  release, v0.10.0 of 21 June 2026, builds on `golang:1.26`, which predates the
  language feature its own generators then have to read. Moving to it changes
  nothing about this fault.
- **Pinning the toolchain down is not available either.** `go.mod` requires
  1.27.4, so an older toolchain refuses the module before any generator starts:
  `go.mod requires go >= 1.27.4 (running go 1.26.0)`. The only toolchain the
  module accepts is the one the old generators cannot read.
- **Nothing in CI runs either generator.** `Chart CRD drift` compares the three
  committed copies of the CRD to each other, not to a regeneration. A CRD stale
  against the Go types therefore passes every check.

## Decisions

- **The development image carries the generators.** It is already built from
  the toolchain `go.mod` names, pinned by digest, and it already carries mockery
  on the same reasoning. Carrying these three as well puts every generator on
  one Go, the one the operator compiles with. It also ends the emulation: the
  published image is amd64 only, which is why both targets named a platform.
- **The three keep the versions the published image carried.** The wrapper at
  0.6.0, `code-generator` at 1.32.3 and controller-gen at 0.17.2 are what the
  image held. The only thing this change varies is the Go that compiles them. `make generate-client` then reproduces the committed `client/k8s` byte
  for byte, which is the evidence that nothing but the toolchain moved. A newer
  generator is a separate question, with its own diff to read.
- **The image installs controller-gen at a version rather than building it from
  a source tarball.** The published image built it with `go build`, which leaves no
  version stamp, so the committed CRD records
  `controller-gen.kubebuilder.io/version: (devel)`. Installing it at its tag
  records `v0.17.2`, which names what wrote the file.
- **The accumulated CRD drift stays out of this change.** A working generator
  shows that the committed CRD predates the Kubernetes libraries the operator
  compiles against. Regenerating it adds `fileKeyRef`, `stopSignal` and
  `restartPolicyRules`. `k8s.io/api` v0.37.2 has those fields and the committed
  copy does not. That CRD ships in the chart, so bringing it
  up to date changes what a cluster accepts and deserves a review of its own.
  Carrying it here would give one pull request two jobs: unbreak generation,
  and change the published API surface.
- **The three wait on the dependency dashboard, and group apart.** mockery and
  golangci-lint travel with the toolchain because each ships as a binary
  carrying its own `golang.org/x/tools`. A newer Go therefore needs a newer
  release of them. The development image compiles these three with the
  toolchain `go.mod` names, so a newer Go needs no newer release of them at
  all. What a move does
  change is what they write. `code-generator` tracks a Kubernetes minor and
  writes `client/k8s`, and controller-tools writes the CRD the chart ships.
  Nothing regenerates either one in CI, so a bad bump merges quietly and
  surfaces when somebody next runs `make generate`. A person decides, the way
  they decide the kind release and the Kubernetes libraries. The three group
  apart from those decisions, because a branch waits for approval when any
  update in it does.
- **No CI job regenerates either one.** That job is the guard this fault argues
  for, as it did for the mocks. It is the obvious follow-up: a
  regenerate-and-diff job in the shape of `Chart CRD drift`. It needs a runner
  to build the development image, which nothing does today. Until it exists,
  the Renovate grouping and this record are what carry the constraint.

## Date

2026-11-18
