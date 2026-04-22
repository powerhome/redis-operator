#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <output-dir>" >&2
  exit 1
fi

OUTPUT_DIR="$1"
CHARTS=(
  "charts/redisoperator"
  "charts/redisfailover-bundle"
)

mkdir -p "${OUTPUT_DIR}"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

sanitize_ref() {
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^0-9a-z-]+/-/g; s/-+/-/g; s/^-+//; s/-+$//'
}

for chart in "${CHARTS[@]}"; do
  chart_name="$(basename "${chart}")"
  chart_copy="${WORK_DIR}/${chart_name}"

  cp -R "${chart}" "${chart_copy}"

  base_version="$(awk '/^version:/ {print $2; exit}' "${chart}/Chart.yaml")"
  base_app_version="$(awk -F'"' '/^appVersion:/ {print $2; exit}' "${chart}/Chart.yaml")"

  package_version="${base_version}"
  package_app_version="${base_app_version}"

  if [[ "${GITHUB_REF_TYPE:-}" != "tag" ]]; then
    ref_name="$(sanitize_ref "${GITHUB_REF_NAME:-dev}")"
    run_number="${GITHUB_RUN_NUMBER:-0}"
    package_version="${base_version}-${ref_name}.${run_number}"
  elif [[ -n "${GITHUB_REF_NAME:-}" ]]; then
    package_app_version="${GITHUB_REF_NAME}"
  fi

  perl -0pi -e "s/^version:\\s*.*/version: ${package_version}/m" "${chart_copy}/Chart.yaml"

  if [[ -n "${package_app_version}" ]]; then
    perl -0pi -e "s/^appVersion:\\s*\".*\"/appVersion: \"${package_app_version}\"/m" "${chart_copy}/Chart.yaml"
  fi

  helm package "${chart_copy}" --destination "${OUTPUT_DIR}"
done
