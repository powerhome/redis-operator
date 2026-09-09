#!/usr/bin/env bash
#
# Validate the redis-operator Helm chart before it is published:
#   1. it lints,
#   2. its bundled CRD is byte-identical to the generated manifests, and
#   3. its appVersion and default image tag name the operator VERSION.
#
# ci.yaml enforces these same invariants as separate gates (chart-test,
# chart-crd-drift, chart-version-check) on the master/PR/operator-`v*` paths.
# The chart-only publish path (helm.yml, `chart-v*` tags / workflow_dispatch)
# does not go through ci.yaml, so it runs this script to get the same coverage
# -- a chart tag cut outside the normal flow cannot publish an invalid or
# drifted artifact after only checking that the tag matches the chart version.
#
# Env overrides (optional):
#   CHART_DIR   path to the chart   (default: charts/redisoperator)
#
set -euo pipefail

CHART_DIR="${CHART_DIR:-charts/redisoperator}"
CRD="databases.spotahome.com_redisfailovers.yaml"

log()  { echo ">> $*"; }
fail() { echo "::error::$*"; exit 1; }

command -v helm >/dev/null || fail "helm is required"

log "helm lint ${CHART_DIR}"
helm lint "${CHART_DIR}"

log "bundled CRD matches generated manifests/${CRD}"
for copy in "manifests/kustomize/base/${CRD}" "${CHART_DIR}/crds/${CRD}"; do
  diff -u "manifests/${CRD}" "${copy}" \
    || fail "${copy} has drifted from manifests/${CRD}; run 'make generate-crd' and commit the result"
done

log "appVersion and image.tag match Makefile VERSION"
version="$(sed -n 's/^VERSION := //p' Makefile | head -1)"
app_version="$(helm show chart "${CHART_DIR}"  | awk '/^appVersion:/ {print $2; exit}')"
img_tag="$(helm show values "${CHART_DIR}" | awk '/^image:/{f=1; next} f&&/^[^[:space:]]/{f=0} f&&/^[[:space:]]+tag:/{print $2; exit}')"
[ -n "${version}" ] || fail "could not read VERSION from Makefile"
[ "${app_version}" = "${version}" ] \
  || fail "chart appVersion (${app_version}) != Makefile VERSION (${version})"
[ "${img_tag}" = "${version}" ] \
  || fail "values.yaml image.tag (${img_tag}) != Makefile VERSION (${version})"

log "chart validation passed (operator VERSION ${version})"
