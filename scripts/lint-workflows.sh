#!/bin/bash

# Lint the GitHub Actions workflow files with actionlint.
#
# actionlint is fetched as a release binary and kept in bin/, rather than run from
# its container image. A container image comes from Docker Hub, which rate limits
# anonymous pulls by address, and CI runners share addresses with everyone else on
# the platform. A rate-limited pull fails a lint job that has nothing to do with
# containers.

set -eu

version="${ACTIONLINT_VERSION:?ACTIONLINT_VERSION must be set}"

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

bin=bin/actionlint-${version}-${os}-${arch}

if [ ! -x "${bin}" ]; then
  archive=actionlint_${version}_${os}_${arch}.tar.gz
  base=https://github.com/rhysd/actionlint/releases/download/v${version}

  workdir="$(mktemp -d)"
  trap 'rm -rf "${workdir}"' EXIT

  echo ">> Fetching actionlint ${version} (${os}/${arch})"
  curl -fsSL -o "${workdir}/${archive}" "${base}/${archive}"
  curl -fsSL -o "${workdir}/checksums.txt" "${base}/actionlint_${version}_checksums.txt"

  # The release publishes one checksum file covering every archive. Verifying
  # against it catches a truncated or corrupted download, which is how this fetch
  # fails in practice.
  ( cd "${workdir}" && grep " ${archive}\$" checksums.txt | verify_sha256 )

  tar -xzf "${workdir}/${archive}" -C "${workdir}" actionlint
  mkdir -p bin
  mv "${workdir}/actionlint" "${bin}"
  chmod +x "${bin}"
fi

echo ">> Linting workflows"
exec "${bin}" -color
