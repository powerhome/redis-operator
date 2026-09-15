#!/usr/bin/env bash
#
# Make every version marker agree with the Makefile.
#
# The operator's version lives in `VERSION` in the Makefile and is repeated in
# eight other places: the chart's appVersion and the image tag it pulls, the
# kustomize version component's tag and label, both plain deployment examples,
# and the install instructions, which name it twice. `Chart version sync` in CI
# holds all of them to the Makefile, so a release that misses one fails rather
# than shipping.
#
# This sets each of them to what the Makefile says. It does not need to know
# what they held, and does not care whether they agreed with each other before.
#
# It does not touch the chart's own version. Two of those markers live in the
# chart, so an operator release is always a chart release, but what the chart's
# next version should be is a judgement, and this makes none. It computes the
# patch bump and says so, and a person sets it.
#
# Forgetting is caught before anything publishes: `make tag-chart` refuses a
# version origin has already tagged.
#
# This writes neither changelog. They need prose, which needs a person.
#
# Usage: prepare-release.sh <operator-version>
#        The Makefile passes $(VERSION), so `make prepare-release` uses what the
#        Makefile declares.

set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-}"
[ -n "${VERSION}" ] || { echo "usage: $(basename "$0") vX.Y.Z" >&2; exit 1; }
case "${VERSION}" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "!! version must look like v4.8.0, got '${VERSION}'" >&2; exit 1 ;;
esac
BARE="${VERSION#v}"

# Set every line matching `pattern` to `line`, whatever it held before.
set_line() {
  local file="$1" pattern="$2" line="$3" n
  n="$(grep -c -- "${pattern}" "${file}" || true)"
  local tmp; tmp="$(mktemp)"
  sed "s|${pattern}|${line}|g" "${file}" > "${tmp}" && mv "${tmp}" "${file}"
  echo "   ${file}  (${n})"
}

echo ">> operator ${VERSION}"
set_line Makefile \
  '^VERSION := .*' "VERSION := ${VERSION}"
set_line charts/redisoperator/Chart.yaml \
  '^appVersion: .*' "appVersion: ${VERSION}"
set_line charts/redisoperator/values.yaml \
  '^  tag: .*' "  tag: ${VERSION}"
set_line manifests/kustomize/components/version/kustomization.yaml \
  'newTag: .*' "newTag: ${VERSION}"
set_line manifests/kustomize/components/version/kustomization.yaml \
  'app.kubernetes.io/version: .*' "app.kubernetes.io/version: ${BARE}"
set_line example/operator/operator.yaml \
  'ghcr.io/powerhome/redis-operator:.*' "ghcr.io/powerhome/redis-operator:${VERSION}"
set_line example/operator/all-redis-operator-resources.yaml \
  'ghcr.io/powerhome/redis-operator:.*' "ghcr.io/powerhome/redis-operator:${VERSION}"
set_line docs/README.md \
  '^REDIS_OPERATOR_VERSION=.*' "REDIS_OPERATOR_VERSION=${VERSION}"

# Read back what is on disk. A marker that has drifted out of the shape above is
# one this never matched, and counting what it wrote would not notice.
echo ">> verifying"
verify() {
  local what="$1" got="$2" want="$3"
  if [ "${got}" != "${want}" ]; then
    echo "!! ${what} is '${got}', expected '${want}'. Set it by hand, and teach this script about it." >&2
    exit 1
  fi
}
verify "Makefile VERSION" "$(sed -n 's/^VERSION := //p' Makefile)" "${VERSION}"
verify "chart appVersion" "$(sed -n 's/^appVersion: //p' charts/redisoperator/Chart.yaml)" "${VERSION}"
verify "values.yaml tag" "$(sed -n 's/^  tag: //p' charts/redisoperator/values.yaml)" "${VERSION}"
verify "kustomize newTag" "$(sed -n 's/^[[:space:]]*newTag: //p' manifests/kustomize/components/version/kustomization.yaml)" "${VERSION}"
verify "kustomize version label" "$(sed -n 's|^[[:space:]]*app.kubernetes.io/version: ||p' manifests/kustomize/components/version/kustomization.yaml)" "${BARE}"
for f in example/operator/operator.yaml example/operator/all-redis-operator-resources.yaml; do
  verify "${f}" "$(sed -n 's|.*ghcr.io/powerhome/redis-operator:||p' "${f}")" "${VERSION}"
done
verify "docs/README.md lines not at ${VERSION}" \
  "$(sed -n 's/^REDIS_OPERATOR_VERSION=//p' docs/README.md | grep -cxv "${VERSION}" || true)" "0"

echo ">> running the chart validation CI runs"
./scripts/validate-chart.sh >/dev/null

echo
echo ">> still to do, by hand:"
echo "   CHANGELOG.md                       a '## [${VERSION}] - $(date +%Y-%m-%d)' heading, and an upgrade note if behaviour changed"
chart_now="$(sed -n 's/^version: //p' charts/redisoperator/Chart.yaml)"
case "${chart_now}" in
  [0-9]*.[0-9]*.[0-9]*) chart_next="${chart_now%.*}.$(( ${chart_now##*.} + 1 ))" ;;
  *) chart_next="a version after ${chart_now}" ;;
esac
echo "   charts/redisoperator/Chart.yaml    set 'version: ${chart_next}', or higher if the chart changed by more"
echo "   charts/redisoperator/CHANGELOG.md  an entry for it, naming operator ${VERSION}"
