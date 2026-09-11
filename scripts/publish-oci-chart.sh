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
# Fail closed: an unparseable image is treated as "not pullable", not skipped.
# Skipping here would let a chart with an empty/malformed image.repository ship
# despite the promise that we never publish a chart nobody can run.
[ -n "${img_repo}" ] && [ -n "${img_tag}" ] \
  || fail "could not determine operator image from ${CHART_DIR}/values.yaml (repository='${img_repo}' tag='${img_tag}'); refusing to publish a chart whose image cannot be verified"
log "Checking operator image ${img_repo}:${img_tag} is pullable"
docker manifest inspect "${img_repo}:${img_tag}" >/dev/null 2>&1 \
  || fail "operator image ${img_repo}:${img_tag} is not pullable yet; publish/build it before releasing the chart"

# --- Helper: HTTP status of a ghcr manifest request, with a supplied token. ----
# Helm rewrites '+' to '_' in OCI tags (SemVer build metadata is not a legal OCI
# tag character), so probe the tag helm actually pushes, not the raw version.
oci_tag="${version//+/_}"

manifest_status() {
  local token="$1"
  curl -s -o /dev/null -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H "Accept: application/vnd.oci.image.manifest.v1+json" \
    "https://${REGISTRY}/v2/${OCI_NAMESPACE}/${name}/manifests/${oci_tag}"
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

# Does the package exist at all yet? A brand-new package starts private, so the
# public-pull verification at the end is advisory only on the run that creates
# it. (Read this before we push, so it reflects the pre-push state.)
tags_status="$(curl -s -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer ${authd_token}" \
  "https://${REGISTRY}/v2/${OCI_NAMESPACE}/${name}/tags/list")"
first_publish=false
[ "${tags_status}" = "404" ] && first_publish=true

status="$(manifest_status "${authd_token}")"
need_push=true
if [ "${status}" = "200" ]; then
  # Already published at this version. A re-run with identical content is fine,
  # but chart `version` is decoupled from the operator and is not gated in CI,
  # so any change that reuses a published version -- an operator bump
  # (appVersion/image.tag) or a chart-only fix (CRD, RBAC, templates, values) --
  # would land here and silently no-op, shipping nothing. Compare the whole
  # packaged chart and only skip the push when it is identical; otherwise fail
  # and ask for a version bump.
  mkdir -p /tmp/oci-published
  rm -rf "/tmp/oci-published/${name}"
  helm pull "${OCI_REPO}/${name}" --version "${version}" --destination /tmp/oci-published --untar
  if ! diff -r "/tmp/oci-published/${name}" "/tmp/oci-local/${name}" >/tmp/oci-chart.diff 2>&1; then
    echo "----- differences between published ${version} and this commit -----"
    cat /tmp/oci-chart.diff
    fail "chart ${name} ${version} is already published with different content. Bump 'version' in charts/redisoperator/Chart.yaml so the change ships under its own chart version."
  fi
  log "Chart ${name} ${version} already published with identical content; will not re-push."
  need_push=false
elif [ "${status}" != "404" ]; then
  fail "unexpected status ${status} probing ${OCI_REPO}/${name}:${version}; refusing to guess"
fi

# --- Push (only when this version is new). ------------------------------------
if [ "${need_push}" = "true" ]; then
  log "Chart ${name} ${version} not yet published; pushing."
  helm push "${local_tgz}" "${OCI_REPO}"
  log "Pushed ${OCI_REPO}/${name}:${version}"
fi

# --- Verify it is publicly pullable. ------------------------------------------
# The only failure that matters to an installer is "released but nobody can
# pull". This runs on both paths -- a fresh push and an already-published
# no-op -- so a package that stays private is caught on every run, not just the
# one that pushed. On the run that first creates the package it is private until
# someone flips visibility, so we surface a task there instead of failing.
anon_token="$(auth_token "")"
anon_status=000
[ -n "${anon_token}" ] && anon_status="$(manifest_status "${anon_token}")"

if [ "${anon_status}" = "200" ]; then
  log "Verified ${OCI_REPO}/${name}:${version} is publicly pullable."
elif [ "${first_publish}" = "true" ] && [ "${need_push}" = "true" ]; then
  echo "::notice title=Make the chart package public::First publish of ${OCI_NAMESPACE}/${name}. Set its visibility to Public (one-time) at https://github.com/orgs/powerhome/packages/container/charts%2F${name}/settings"
else
  fail "${OCI_REPO}/${name}:${version} is published but not publicly pullable (status ${anon_status}); set the ghcr package visibility to public"
fi
