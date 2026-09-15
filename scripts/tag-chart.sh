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
# This refuses a version an operator release already published. An operator tag
# publishes the chart too, at whatever version Chart.yaml declared then. Tagging
# that version again would publish nothing new, because the publish script
# compares the packaged chart before pushing. The tag would still claim a
# chart-only release that never happened.

set -euo pipefail

cd "$(dirname "$0")/.."

CHART_VERSION="$(sed -n 's/^version: //p' charts/redisoperator/Chart.yaml)"
APP_VERSION="$(sed -n 's/^appVersion: //p' charts/redisoperator/Chart.yaml)"
TAG="chart-v${CHART_VERSION}"

# The operator release that ships this appVersion publishes the chart too. If
# that release is already tagged, and declared this same chart version, it has
# published it.
#
# The tag has to come from the remote. A clone configured not to follow tags,
# which this one is, can be missing a release someone else cut, and a check that
# reads only local tags would pass on a version already published. Fetch the one
# tag this reasons about, and say so if the remote could not be reached rather
# than reporting a check that did not happen.
reachable=true
if ! git fetch --quiet origin "refs/tags/${APP_VERSION}:refs/tags/${APP_VERSION}" 2>/dev/null; then
  git ls-remote --exit-code --tags origin "${APP_VERSION}" >/dev/null 2>&1 || reachable=$?
  [ "${reachable}" = "2" ] && reachable=true   # reached it; the tag is simply not there
fi
if [ "${reachable}" != "true" ] && ! git rev-parse -q --verify "refs/tags/${APP_VERSION}" >/dev/null; then
  echo "!! could not reach origin to check whether operator release ${APP_VERSION} is tagged." >&2
  echo "   Fetch tags and try again, or confirm by hand that chart ${CHART_VERSION} is unpublished." >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/${APP_VERSION}" >/dev/null; then
  published_then="$(git show "${APP_VERSION}:charts/redisoperator/Chart.yaml" 2>/dev/null | sed -n 's/^version: //p' || true)"
  if [ "${published_then}" = "${CHART_VERSION}" ]; then
    echo "!! chart ${CHART_VERSION} was published by operator release ${APP_VERSION}, which is already tagged." >&2
    echo "   A chart-only release needs a version of its own. Bump 'version' in" >&2
    echo "   charts/redisoperator/Chart.yaml, leaving appVersion alone." >&2
    exit 1
  fi
fi

git tag "${TAG}"
echo "tagged ${TAG}. Push it to publish: git push origin ${TAG}"
