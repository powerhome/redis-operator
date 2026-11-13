#!/bin/bash

# Test the chart with the helm the Makefile names.
#
# helm is fetched as a release binary and kept in bin/, the way
# scripts/lint-workflows.sh fetches actionlint. A helm already on the machine is
# not used even when there is one: `helm lint` and `helm template` change what
# they accept and what they render between releases, so a chart that passes
# against one helm can fail against another, and the version that gates a merge
# should be the version a contributor runs.

set -eu

version="${HELM_VERSION:?HELM_VERSION must be set}"
chart=charts/redisoperator

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *)      echo "Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64)        arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *)             echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

# Linux carries sha256sum, macOS carries shasum, and neither carries the other.
verify_sha256() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum -c -
  else
    shasum -a 256 -c -
  fi
}

helm=bin/helm-${version}-${os}-${arch}

if [ ! -x "${helm}" ]; then
  archive=helm-v${version}-${os}-${arch}.tar.gz

  workdir="$(mktemp -d)"
  trap 'rm -rf "${workdir}" "${helm}.$$"' EXIT

  echo ">> Fetching helm ${version} (${os}/${arch})"
  curl -fsSL -o "${workdir}/${archive}" "https://get.helm.sh/${archive}"
  curl -fsSL -o "${workdir}/${archive}.sha256sum" "https://get.helm.sh/${archive}.sha256sum"

  # Catches a truncated or corrupted download, which is how this fetch fails in
  # practice.
  ( cd "${workdir}" && verify_sha256 < "${archive}.sha256sum" )

  # The archive holds the binary under a directory named for the platform.
  tar -xzf "${workdir}/${archive}" -C "${workdir}" "${os}-${arch}/helm"
  # The binary comes out executable, and a move across filesystems copies
  # rather than renames, so a partial copy is executable too. Staging it beside
  # its destination and renaming within bin/ is atomic, which keeps the name
  # the test runs from ever naming an incomplete file.
  mkdir -p bin
  mv "${workdir}/${os}-${arch}/helm" "${helm}.$$"
  chmod +x "${helm}.$$"
  mv "${helm}.$$" "${helm}"
fi

echo ">> Testing chart ${chart} with helm ${version}"

"${helm}" lint ${chart}
"${helm}" template ${chart}

echo "> Chart OK"
