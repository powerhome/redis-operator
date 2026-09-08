# CIR-004: Sync the chart to the operator, and publish it to ghcr.io as OCI

## Intent

Keep the Helm chart's operator-derived content — its bundled CRD, its default
image, its appVersion — matching the operator it ships, and publish every chart
release to ghcr.io as an OCI artifact under `powerhome/charts`, without a person
hand-editing the chart to track the operator and without a release path that can
silently lag.

Three drift surfaces were identified: the CRD (generated), the image and
appVersion (chosen at release), and the RBAC ClusterRole (hand-maintained,
reviewed on change). This record covers the first two and the publish path; the
RBAC surface stays a review-on-change checklist.

## Behavior

- GIVEN the api/ types change and nobody ran `make generate-crd`
- WHEN CI runs
- THEN `chart-crd-drift` regenerates the CRD and fails on the diff, so a stale
  manifest or a stale chart copy cannot merge

- GIVEN a release commit sets `Makefile VERSION` but not the chart's appVersion
  or `image.tag`
- WHEN CI runs
- THEN `chart-version-check` fails, naming the file to fix

- GIVEN an operator release tag `vX.Y.Z` is pushed
- WHEN the operator image has been built and pushed
- THEN the chart is packaged from that same commit and pushed to
  `oci://ghcr.io/powerhome/charts/redis-operator`, and `helm pull` of that
  version succeeds

- GIVEN a chart-only fix with no operator release
- WHEN a `chart-vX.Y.Z` tag is pushed (or the workflow is run by hand)
- THEN the chart publishes at its own version, pointing at the operator image
  it already names

- GIVEN a version already present in the registry
- WHEN either publish path runs again
- THEN it is a no-op, distinguished from a registry error, which fails loudly

## Constraints

- **The release version is an input, not an output.** A person chooses it and
  writes it into `Makefile VERSION`; the tag is cut from a commit that already
  contains it. So the chart's appVersion and `image.tag` are set in that same
  commit and CI *gates* them, the same generate-then-gate shape used for the
  CRD. Nothing is computed and written back to a branch.
- **appVersion carries the `v`.** The operator image is tagged `type=semver,
  pattern={{raw}}`, which publishes `vX.Y.Z` and no bare `X.Y.Z`. The chart's
  deployment falls back to `.Chart.AppVersion` when `image.tag` is empty, so
  appVersion must be `vX.Y.Z` or that fallback resolves an image that does not
  exist. appVersion, `image.tag`, and `Makefile VERSION` are therefore all
  `vX.Y.Z`, and the gate checks equality.
- **Publish only where it works.** ghcr.io has native Helm OCI support and
  pushes with the built-in `GITHUB_TOKEN`. It is the only chart distribution
  path this repository provides.

## Decisions

- **Chart `version` moves independently of the operator.** Lock-stepping the
  chart version to the operator leaves no number to turn for a chart-only fix
  (a bad RBAC rule, a template bug, a values default): the only ways out are a
  fake operator release or a hand-edit that then collides with the automation
  and publishes a chart still pointing at the old image. appVersion already
  answers "which operator does this install", so the chart's own SemVer is free
  to move, and chart-only releases publish from `helm.yml` on a `chart-v*` tag.

- **Rejected: a CI job that bumps the chart and pushes to master.** The first
  design recomputed the version from the tag, wrote it back to Chart.yaml and
  values.yaml, and pushed to master to trigger publishing. Two faults sank it.
  It restated a fact `Makefile VERSION` already held, in three hand-maintained
  regexes with no test — the same hand-maintenance the change exists to end. And
  **a push authenticated with `GITHUB_TOKEN` creates no workflow runs**
  (`workflow_dispatch` and `repository_dispatch` are the only exceptions), so
  the bump landed on master and published nothing while the run stayed green.
  Setting the version in source and gating it needs no write to the default
  branch, no token question, no concurrency or rebase handling, and fails red
  when someone forgets instead of failing silent.

- **Rejected: keeping the HTTP Helm repository (chart-releaser + GitHub Pages).**
  It has never served for this fork and cannot: the organization's GitHub Pages
  is given to the techatpower site, so this repo's Pages output redirects to
  tech.powerhrg.com and `helm repo add https://powerhome.github.io/redis-operator`
  404s on index.yaml. The job has packaged charts into a branch nobody can read
  since 2023; its one published release, Chart-3.3.0, carries upstream's
  numbers. Removing it (the `release` job and `charts/chart-release-config.yaml`)
  also dissolves the "both paths must not diverge" problem the earlier design
  spent effort guaranteeing: with one path there is nothing to keep in step. The
  gh-pages branch and the Chart-3.3.0 release are left in place; deleting already
  published artifacts is a separate decision.

- **Rejected: publishing the chart on push to master.** A master-push trigger
  would publish before the operator image tag exists (the release commit merges,
  the tag is cut afterward), so it needs either a bot push — see above — or a
  blocking image-pullable precheck that fails every release commit until someone
  re-runs it. Publishing on the operator tag with `needs: dockerhub-image` gets
  the ordering for free and from a real event.

- **Accepted: regenerating the CRD in CI rather than diffing two committed
  copies.** Comparing the chart copy against the committed manifest passes even
  when both lag the api/ types. `chart-crd-drift` runs `make generate-crd` (with
  `DOCKER_INTERACTIVE=` so it works on a TTY-less runner) and requires a clean
  `git diff`, catching the stale-manifest case too.

- **Accepted: the first publish surfaces a task instead of failing.** A new ghcr
  package is private until someone flips its visibility, so the publish script's
  public-pull check emits a `::notice::` with the settings link on first publish
  and only fails afterward, when a private result is a real regression.

## Verification

- `helm lint` and `helm template` pass; the rendered Deployment resolves
  `powerhome/redis-operator:v4.6.0`, a tag that exists.
- The bundled CRD is byte-identical to `manifests/` and the kustomize base after
  `make generate-crd`.
- The publish script's version probe was reasoned against ghcr's manifest and
  tags endpoints: 200 is a no-op, 404 packages and pushes, anything else fails
  rather than masquerading as "already there".

Not yet validated against a real release tag; that happens the first time an
operator `vX.Y.Z` is cut after this lands, including the one-time package
visibility flip.

## Date

2026-09-08
