#!/usr/bin/env bash
#
# Tag a chart-only release.
#
# The tag name comes from `version` in Chart.yaml. The chart's version lives
# there rather than in the Makefile because it moves without the operator.
#
# `helm.yml` rejects a tag naming a version the chart does not declare. Building
# the name from that file means the two always agree.
#
# Every published chart version carries a tag, including one an operator release
# published, so that anyone running a chart can read the source it was built
# from. Pushing a tag for a version already published republishes nothing: the
# publish script compares the packaged chart and skips when it matches.
#
set -euo pipefail

cd "$(dirname "$0")/.."

CHART_VERSION="$(sed -n 's/^version: //p' charts/redisoperator/Chart.yaml)"
TAG="chart-v${CHART_VERSION}"

# A tag that already exists means the chart's version was not moved. On an
# operator release that is a mistake: the release changes what the chart
# installs, so its version changes with it.
#
# Ask origin, not the local tags. A clone configured not to follow tags, which
# this one is, can be missing a version someone else released. One request,
# named: 0 found it, 2 reached origin and did not, anything else could not reach
# origin at all.
found=0
git ls-remote --exit-code --tags origin "${TAG}" >/dev/null 2>&1 || found=$?
case "${found}" in
  0)
    echo "!! ${TAG} is already tagged on origin, so the chart's version has not moved." >&2
    echo "   Bump 'version' in charts/redisoperator/Chart.yaml." >&2
    exit 1
    ;;
  2) ;;  # reached origin; the tag is not there
  *)
    echo "!! could not reach origin to check whether ${TAG} is already released." >&2
    echo "   Try again with a connection, or tag by hand if you are certain." >&2
    exit 1
    ;;
esac

if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  echo "!! ${TAG} already exists locally but not on origin." >&2
  echo "   Push it, or delete it and tag again: git tag -d ${TAG}" >&2
  exit 1
fi

# A release tags the commit that set the chart's version, which is not necessarily what
# origin's master points at now. The two are the same commit only when nothing
# merges between the release commit landing and the tag being cut, and a
# release that waits on a person cannot assume that.
#
# CI cannot make this check. It compares the tag's name to the chart's version, and
# the chart's version reads the same on every commit after the release one, so a tag cut
# from a later commit passes while shipping changes no changelog describes.
git fetch --quiet origin master
head="$(git rev-parse HEAD)"
master="$(git rev-parse FETCH_HEAD)"

if ! git merge-base --is-ancestor "${head}" "${master}"; then
  echo "!! HEAD is not merged into origin's master." >&2
  echo "   HEAD           ${head}" >&2
  echo "   origin/master  ${master}" >&2
  echo "   A release tags a reviewed commit. Merge it first, then pull." >&2
  exit 1
fi

release="$(git log -1 --format=%H -G'^version: ' "${master}" -- charts/redisoperator/Chart.yaml)"
if [ -z "${release}" ]; then
  echo "!! could not find the commit that set the chart's version." >&2
  exit 1
fi

if [ "${head}" != "${release}" ]; then
  echo "!! HEAD is not the commit that set the chart's version to ${CHART_VERSION}." >&2
  echo "   HEAD          ${head}" >&2
  echo "   release       ${release}  $(git log -1 --format=%s "${release}")" >&2
  echo "   Tagging anything later ships changes the changelog does not describe." >&2
  echo "   Tag the release commit:" >&2
  echo "     git checkout ${release} && make tag-chart && git checkout -" >&2
  exit 1
fi

git tag "${TAG}"
echo "tagged ${TAG}. Push it to publish: git push origin ${TAG}"
