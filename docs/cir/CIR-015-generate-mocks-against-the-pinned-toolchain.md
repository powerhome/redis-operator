# CIR-015: Generate mocks against the pinned toolchain

## Intent

Keep `make generate-mocks` able to run, so that changing an interface is an
ordinary change rather than one that has to work around its own tooling.

mockery reads the packages it generates from, so it carries its own copy of
`golang.org/x/tools`. A release whose copy predates the toolchain in `go.mod`
cannot read what that toolchain compiles. It stops on the first package with
`internal error: package "fmt" without types was imported from
"github.com/spotahome/redis-operator/log"` and writes nothing.

That is where the repository stood. The `go` directive moved to 1.27.4 while the
development image carried mockery 2.52.4, which pins `golang.org/x/tools`
v0.30.0. The toolchain move passed every check, because no check runs mockery.
The break surfaced only when someone changed an interface and asked for the
mocks to be rewritten.

## Behavior

- GIVEN a checkout on the toolchain `go.mod` names
- WHEN `make generate-mocks` runs
- THEN every mock in `mocks/` is rewritten, and the only difference from the
  committed copies is whatever the interfaces changed

- GIVEN a development image built without a mockery release named
- WHEN the image builds
- THEN the build fails at that step, rather than installing a release nobody
  chose

- GIVEN a new Go toolchain release
- WHEN Renovate proposes it
- THEN mockery travels in the same pull request, the way golangci-lint does

- GIVEN mockery alone has a new release
- WHEN Renovate proposes it
- THEN it still travels as part of the toolchain group, because the group is
  what keeps the pair together

- GIVEN a committed mock that has drifted from its interface
- WHEN CI runs
- THEN nothing fails, because no job regenerates them

## Constraints

- **The Makefile holds one pin per tool.** golangci-lint, helm and kind each
  live there with a `# renovate:` comment above them, read by
  `customManagers:makefileVersions`, and
  [CIR-013](CIR-013-run-the-same-tests-locally-and-in-ci.md) is why. A pin in a
  Dockerfile `ARG` is read by nothing, which is how this one reached eighteen
  months of age without anyone choosing it.
- **The directives live in `mocks/doc.go` as command-line flags.**
  `mockery --output ... --dir ... --name ...`, one per interface.
- **Nothing in CI runs mockery.** A stale golangci-lint fails the lint job
  loudly. A stale mockery is silent until a contributor needs it.

## Decisions

- **mockery v2.53.8 rather than v3.9.1.** v3 is the current line and carries the
  same `golang.org/x/tools` v0.51.0, so it would fix the fault equally well. It
  also removes the command-line flags `mocks/doc.go` uses, in favour of a
  `.mockery.yml` describing packages. Rewriting seven directives into a
  configuration file is a change with its own review. Bundling it here would
  give one pull request two jobs: unbreak generation, and rewrite how generation
  reads its own configuration. v2.53.8 unbreaks it and changes nothing else:
  regenerating against it rewrites one line per file, the version banner. v2 is
  on its way out and warns about it on every run. The move to v3 is a question
  this defers rather than answers.
- **The pin moves to the Makefile, and the Dockerfile `ARG` loses its default.**
  Two defaults that can disagree is the shape of the original fault. The Makefile
  passes the pin to `docker buildx bake` the way it already passes `uid`, and a
  build that names no release fails rather than guessing.
- **mockery joins the `go toolchain` Renovate group.** golangci-lint is in that
  group for the same reason: it reads what the toolchain compiles, so the two
  have to move together. Grouping is weaker here than there, because no job
  fails when they come apart. That weakness argues for the group, not against
  it.
- **No CI job regenerates the mocks.** That job is the guard this fault argues
  for, and it is the obvious follow-up. It would be a regenerate-and-diff job in
  the shape of `Chart CRD drift`, whose own comment already names the same
  improvement as pending for the CRDs. It needs a runner to build the development image, which
  nothing does today. So it is a change to measure rather than one to add here.
  Until it exists, the Renovate grouping and this record are what carry the
  constraint.

## Date

2026-11-17
