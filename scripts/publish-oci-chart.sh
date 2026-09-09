#!/usr/bin/env bash
#
# Package the redis-operator Helm chart and publish it to ghcr.io as an OCI
# artifact. Idempotent: an already-published version is a no-op. The caller must
# already be logged in to ghcr.io (`helm registry login`) and must export
# GITHUB_TOKEN and GITHUB_ACTOR so the "already published?" probe can read a
# package that is still private (e.g. before its first public release).
#
# Env overrides (all optional):
#   CHART_DIR      path to the chart            (default: charts/redisoperator)
#   REGISTRY       ghcr registry host           (default: ghcr.io)
#   OCI_NAMESPACE  org/subpath under the host   (default: powerhome/charts)
#
set -euo pipefail

CHART_DIR="${CHART_DIR:-charts/redisoperator}"
REGISTRY="${REGISTRY:-ghcr.io}"
OCI_NAMESPACE="${OCI_NAMESPACE:-powerhome/charts}"
OCI_REPO="oci://${REGISTRY}/${OCI_NAMESPACE}"

log()  { echo ">> $*"; }
fail() { echo "::error::$*"; exit 1; }

command -v helm >/dev/null || fail "helm is required"
command -v jq   >/dev/null || fail "jq is required"

name="$(helm show chart "${CHART_DIR}" | awk '/^name:/ {print $2; exit}')"
version="$(helm show chart "${CHART_DIR}" | awk '/^version:/ {print $2; exit}')"
app_version="$(helm show chart "${CHART_DIR}" | awk '/^appVersion:/ {print $2; exit}')"
[ -n "${name}" ] && [ -n "${version}" ] || fail "could not read chart name/version from ${CHART_DIR}"

# --- Precondition: the operator image this chart points at must exist. ---------
# A chart that references an image nobody can pull is worse than no chart, so we
# refuse to publish one. This is what keeps the chart from getting ahead of the
# operator without a bot pushing to master.
img_repo="$(helm show values "${CHART_DIR}" | awk '/^image:/{f=1; next} f&&/^[^[:space:]]/{f=0} f&&/^[[:space:]]+repository:/{print $2; exit}')"
img_tag="$(helm show values "${CHART_DIR}"  | awk '/^image:/{f=1; next} f&&/^[^[:space:]]/{f=0} f&&/^[[:space:]]+tag:/{print $2; exit}')"
img_tag="${img_tag:-${app_version}}"
if [ -n "${img_repo}" ] && [ -n "${img_tag}" ]; then
  log "Checking operator image ${img_repo}:${img_tag} is pullable"
  docker manifest inspect "${img_repo}:${img_tag}" >/dev/null 2>&1 \
    || fail "operator image ${img_repo}:${img_tag} is not pullable yet; publish/build it before releasing the chart"
else
  log "WARNING: could not determine operator image from values; skipping image precheck"
fi

# --- Helper: HTTP status of a ghcr manifest request, with a supplied token. ----
manifest_status() {
  local token="$1"
  curl -s -o /dev/null -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H "Accept: application/vnd.oci.image.manifest.v1+json" \
    "https://${REGISTRY}/v2/${OCI_NAMESPACE}/${name}/manifests/${version}"
}

auth_token() {
  # -u makes this work for a private package; anonymous when creds are empty.
  curl -s ${1:+-u "$1"} \
    "https://${REGISTRY}/token?scope=repository:${OCI_NAMESPACE}/${name}:pull&service=${REGISTRY}" \
    | jq -r '.token // empty'
}

# --- Package the chart once, up front. ----------------------------------------
# The packaged (.helmignore-filtered) form is what we both compare against an
# existing version and push for a new one, so build it once and reuse it.
mkdir -p /tmp/oci-pkg /tmp/oci-local
rm -rf "/tmp/oci-local/${name}"
helm package "${CHART_DIR}" --destination /tmp/oci-pkg >/dev/null
local_tgz="/tmp/oci-pkg/${name}-${version}.tgz"
tar -xzf "${local_tgz}" -C /tmp/oci-local

# --- Is this version already published? ---------------------------------------
# Distinguish "already there" (200) from "not there" (404) from "cannot tell"
# (anything else) so a broken token or an outage never masquerades as a no-op.
authd_token="$(auth_token "${GITHUB_ACTOR:-}:${GITHUB_TOKEN:-}")"
[ -n "${authd_token}" ] || fail "could not obtain an authenticated ghcr token; check GITHUB_TOKEN/GITHUB_ACTOR"

status="$(manifest_status "${authd_token}")"
if [ "${status}" = "200" ]; then
  # Already published at this version. A re-run with identical content is fine,
  # but chart `version` is decoupled from the operator and is not gated in CI,
  # so any change that reuses a published version -- an operator bump
  # (appVersion/image.tag) or a chart-only fix (CRD, RBAC, templates, values) --
  # would land here and silently no-op, shipping nothing. Compare the whole
  # packaged chart and only no-op when it is identical; otherwise fail and ask
  # for a version bump.
  mkdir -p /tmp/oci-published
  rm -rf "/tmp/oci-published/${name}"
  helm pull "${OCI_REPO}/${name}" --version "${version}" --destination /tmp/oci-published --untar
  if diff -r "/tmp/oci-published/${name}" "/tmp/oci-local/${name}" >/tmp/oci-chart.diff 2>&1; then
    log "Chart ${name} ${version} already published with identical content; nothing to do."
    exit 0
  fi
  echo "----- differences between published ${version} and this commit -----"
  cat /tmp/oci-chart.diff
  fail "chart ${name} ${version} is already published with different content. Bump 'version' in charts/redisoperator/Chart.yaml so the change ships under its own chart version."
elif [ "${status}" != "404" ]; then
  fail "unexpected status ${status} probing ${OCI_REPO}/${name}:${version}; refusing to guess"
fi
log "Chart ${name} ${version} not yet published; pushing."

# Does the package exist at all? A brand-new package starts private, so the
# public-pull verification below is advisory on the very first publish only.
tags_status="$(curl -s -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer ${authd_token}" \
  "https://${REGISTRY}/v2/${OCI_NAMESPACE}/${name}/tags/list")"
first_publish=false
[ "${tags_status}" = "404" ] && first_publish=true

# --- Push. --------------------------------------------------------------------
helm push "${local_tgz}" "${OCI_REPO}"
log "Pushed ${OCI_REPO}/${name}:${version}"

# --- Verify it is publicly pullable. ------------------------------------------
# The only failure that matters to an installer is "pushed but nobody can pull".
# On the first publish the package is private until someone flips visibility, so
# we surface a task instead of failing; afterwards, a private result is a real
# regression (or a chart rename creating a fresh private package).
anon_token="$(auth_token "")"
anon_status=000
[ -n "${anon_token}" ] && anon_status="$(manifest_status "${anon_token}")"

if [ "${anon_status}" = "200" ]; then
  log "Verified ${OCI_REPO}/${name}:${version} is publicly pullable."
elif [ "${first_publish}" = "true" ]; then
  echo "::notice title=Make the chart package public::First publish of ${OCI_NAMESPACE}/${name}. Set its visibility to Public (one-time) at https://github.com/orgs/powerhome/packages/container/charts%2F${name}/settings"
else
  fail "${OCI_REPO}/${name}:${version} was pushed but is not publicly pullable (status ${anon_status}); check the ghcr package visibility"
fi
