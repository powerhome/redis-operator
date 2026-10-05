#!/bin/bash

# Why the tests run inside the kind node: docs/cir/CIR-012-run-integration-tests-on-kind.md

set -eu

kind_version="${KIND_VERSION:?KIND_VERSION must be set}"
node_image="${KIND_NODE_IMAGE:?KIND_NODE_IMAGE must be set}"
cluster="${KIND_CLUSTER_NAME:-redis-operator-integration}"
node="${cluster}-control-plane"
crd=manifests/databases.spotahome.com_redisfailovers.yaml

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

kind=bin/kind-${kind_version}-${os}-${arch}
cluster_created=

workdir="$(mktemp -d)"
cleanup() {
  if [ -n "${cluster_created}" ]; then
    echo ">> Deleting kind cluster ${cluster}"
    "${kind}" delete cluster --name "${cluster}" --kubeconfig "${workdir}/kubeconfig" || true
  fi
  rm -rf "${workdir}"
}
trap cleanup EXIT

verify_sha256() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum -c -
  else
    shasum -a 256 -c -
  fi
}

fetch_kind() {
  local asset=kind-${os}-${arch}
  local base=https://github.com/kubernetes-sigs/kind/releases/download/v${kind_version}

  echo ">> Fetching kind ${kind_version} (${os}/${arch})"
  curl -fsSL -o "${workdir}/${asset}" "${base}/${asset}"
  curl -fsSL -o "${workdir}/${asset}.sha256sum" "${base}/${asset}.sha256sum"
  ( cd "${workdir}" && verify_sha256 < "${asset}.sha256sum" )

  mkdir -p bin
  mv "${workdir}/${asset}" "${kind}"
  chmod +x "${kind}"
}

build_tests_for_node() {
  local node_arch
  node_arch="$(docker version --format '{{.Server.Arch}}')"

  echo ">> Building integration tests (linux/${node_arch})"
  mkdir -p "${workdir}/tests"
  # On a Linux host this links against the host's C library, which the node
  # image's must be at least as new as, or the binary fails to start.
  GOOS=linux GOARCH="${node_arch}" \
    go test -c -tags integration -o "${workdir}/tests/" ./test/integration/...
}

create_cluster() {
  echo ">> Creating kind cluster ${cluster} from ${node_image}"
  cluster_created=yes
  # Without --kubeconfig, kind writes this cluster into the caller's
  # ~/.kube/config and makes it the current context.
  "${kind}" create cluster \
    --name "${cluster}" \
    --image "${node_image}" \
    --kubeconfig "${workdir}/kubeconfig" \
    --wait 5m
}

in_node() {
  docker exec -i "${node}" "$@"
}

install_crd() {
  echo ">> Installing the RedisFailover CRD"
  in_node kubectl --kubeconfig /etc/kubernetes/admin.conf create -f - < "${crd}"
  wait_until_redisfailovers_are_served
}

wait_until_redisfailovers_are_served() {
  local attempt
  for attempt in $(seq 60); do
    if in_node kubectl --kubeconfig /etc/kubernetes/admin.conf get redisfailovers --all-namespaces > /dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "The API server did not serve RedisFailovers within 60 seconds" >&2
  return 1
}

install_kubeconfig_where_tests_read_it() {
  in_node sh -c 'mkdir -p "${HOME}/.kube" && cp /etc/kubernetes/admin.conf "${HOME}/.kube/config"'
}

run_tests_in_node() {
  docker cp "${workdir}/tests" "${node}:/integration-tests"

  local test_binary
  for test_binary in "${workdir}"/tests/*.test; do
    echo ">> Running $(basename "${test_binary}")"
    in_node "/integration-tests/$(basename "${test_binary}")" -test.v
  done
}

[ -x "${kind}" ] || fetch_kind
build_tests_for_node
create_cluster
install_crd
install_kubeconfig_where_tests_read_it
run_tests_in_node
