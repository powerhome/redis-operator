# Releasing

This repository publishes two things, and each has its own version.

- **The operator**, as a container image. Its version is `VERSION` in the
  `Makefile`.
- **The Helm chart** that installs it. Its version is `version` in
  `charts/redisoperator/Chart.yaml`.

The two are unrelated. The chart's version moves whenever the chart changes, so
a chart-only fix can ship without inventing an operator release. A chart at
`4.8.1` installing operator `v4.7.0` is an ordinary state, and so is a chart at
`2.1.7`. `docs/cir/CIR-004-chart-sync-and-oci-publish.md` records why.

One field records the link between them. `appVersion` in `Chart.yaml` names the
operator a given chart installs. It holds the operator's version, not a third
one, and CI keeps the two equal.

So two versions, written in three places:

| Where                        | Holds                              | Checked against                                       |
|------------------------------|------------------------------------|-------------------------------------------------------|
| `VERSION` in `Makefile`      | the operator's version             | nothing. Eight other markers are checked against it   |
| `appVersion` in `Chart.yaml` | the operator's version, again      | `Makefile` VERSION                                    |
| `version` in `Chart.yaml`    | the chart's version                | nothing                                               |

## Releasing the operator

The chart goes with it: an operator release changes what the chart installs, so
the chart publishes from the same tag.

1. **Set the two versions.**

   - `VERSION` in the `Makefile` is the operator's.
   - `version` in `charts/redisoperator/Chart.yaml` is the chart's. It changes
     because the chart's content changes, which an operator release always does:
     the chart now installs a different operator.

   Then make everything else agree:

   ```
   make prepare-release
   ```

   This sets the eight other markers to whatever `VERSION` says: the chart's
   `appVersion` and image tag, the kustomize version component's tag and label,
   both plain deployment examples, and the install instructions in
   `docs/README.md`, which name it twice. It does not touch the chart's version.

   It stops if a marker does not end up at the new version. That happens when
   someone adds or renames one and the script has not been taught about it. Set
   it by hand, then teach the script.

2. **Write the changelogs.** The script does not write them. Prose needs a
   person.

   - `CHANGELOG.md` gets a `## [v4.8.0] - YYYY-MM-DD` heading above the entries
     that were under `Unreleased`, which stays and is left empty. Add an
     `### Upgrade note` when behaviour changes in a way anyone upgrading has
     to know about.
   - `charts/redisoperator/CHANGELOG.md` gets an entry naming the operator
     version and pointing at the repository changelog. Do not retell the
     operator's release there.

3. **Open a pull request, get it reviewed, merge it.** Title it
   `Prepare release - operator vX.Y.Z, chart A.B.C`, naming both versions, since
   the release moves both. The chart's carries no `v`: only its tag does.

   The release commit is the one the tag has to name, so nothing else may merge
   between this and step 4.

4. **Tag the release commit and push the tag.**

   ```
   git checkout master && git pull --ff-only
   make tag
   ```

   `make tag` runs `tag-operator` and `tag-chart`. Each reads its version from
   where that version is declared, and prints the push command with the tag it
   created. Nothing is published until those pushes run.

   The chart gets a tag even though it publishes from the operator's, so that
   anyone running a chart can read the source it was built from. Pushing it
   republishes nothing: the publish script compares the packaged chart and skips
   when it matches. The two publish paths share a concurrency group, so they
   serialise.

   It refuses three things. A version origin has already tagged, a commit that
   is not merged into origin's master, and a commit that is not the one that set
   `VERSION`.

   That last check matters because CI cannot make it. CI compares the tag's name
   to `VERSION`, and `VERSION` reads the same on every commit after the release
   one, so a tag cut from a later commit passes while shipping changes the
   changelog does not describe.

   **If something merges between step 3 and here, `git pull` moves you past the
   release commit and the tag targets refuse.** That is the point: the release
   is the commit that set `VERSION`, not whatever master points at now. Tag it
   where it is, and the refusal prints the command:

   ```
   git checkout <release commit> && make tag && git checkout -
   ```

   Like `make tag-chart`, it asks origin about tags instead of reading local
   ones, and refuses when origin cannot be reached.

   `make tag-chart` stops if `chart-v<version>` already exists, which means the
   chart's version was not moved. An operator release always changes what the
   chart installs, so its version moves with it.

The tag runs the full pipeline. `dockerhub-image` builds and pushes the operator
image to Docker Hub and ghcr, and `latest` moves to it. `chart-oci-publish` then
publishes the chart, but only once the image exists and the chart's own gates
pass. A published chart never names an image that is not there.

## Releasing the chart alone

For a chart fix that needs no new operator: a template, a value, an RBAC rule.

1. Bump `version` in `charts/redisoperator/Chart.yaml`. Leave `appVersion` and
   `values.yaml`'s `image.tag` alone, since the operator is not moving.
2. Add an entry to `charts/redisoperator/CHANGELOG.md`.
3. Open a pull request, get it reviewed, merge it. Title it
   `Prepare release - chart A.B.C`. No operator version, since it is not moving.
4. Tag and push:

   ```
   git checkout master && git pull --ff-only
   make tag-chart
   ```

   `make tag-chart` reads the version from `Chart.yaml` and prints the push
   command with the tag it created, so the tag cannot name a version the chart
   does not declare and there is no number to copy.

   It refuses the same three things `make tag-operator` does, against the
   chart's own version: one origin has already tagged, which means the version
   was not moved; a commit not merged into origin's master; and a commit that is
   not the one that set `version` in `Chart.yaml`. `helm.yml` cannot make those
   last two: it compares the tag's name to the chart's version, and both read the
   same on any commit after the bump.

   It asks origin about tags rather than reading local ones, since a clone can
   be configured not to follow them, and refuses when origin cannot be reached.

A push to `master` publishes nothing. The chart publishes from a `chart-v*` tag.

## What CI refuses

- **`appVersion` or `values.yaml` `image.tag` not equal to `Makefile` VERSION.**
  The chart would install an operator other than the one being released.
- **A version marker in the manifests, examples or install instructions not
  equal to `Makefile` VERSION.** Someone following those installs a different
  version than the release.
- **A release tag that is not `Makefile` VERSION.** The tag was cut from the
  wrong commit.
- **A prerelease tag**, anything with a `-` in it. Chart publishing has no
  prerelease path and would ship it as an ordinary version.
- **A `chart-v*` tag naming a version `Chart.yaml` does not declare.**
- **A chart version already published with different content.** Bump the chart
  version: publishing is idempotent on that version and would otherwise ship
  nothing.
- **A chart version that has not moved.** `make tag-chart` refuses before the
  tag exists.
- **A tag on a commit that is not the one that set the version, or that is not
  merged into origin's master.** Both `make tag-operator` and `make tag-chart`
  refuse before the tag exists. Neither CI workflow can check this, since every
  version marker reads the same on every commit after the one that set it.
