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

git tag "${TAG}"
echo "tagged ${TAG}. Push it to publish: git push origin ${TAG}"
