VERSION := v5.1.0

# Name of this service/application
SERVICE_NAME := redis-operator

# Docker image name for this project
IMAGE_NAME := powerhome/$(SERVICE_NAME)

# Repository url for this project
REPOSITORY := $(IMAGE_NAME)

# Get docker path or an empty string
DOCKER := $(shell command -v docker)

# Get the unix user id for the user running make (to be used by docker bake)
UID := $(shell id -u)

GIT_COMMIT=$(shell git rev-parse HEAD)
IMAGE_TAG := $(GIT_COMMIT)
ifneq ($(shell git status --porcelain),)
	IMAGE_TAG := $(IMAGE_TAG)-dirty
endif

PROJECT_PACKAGE := github.com/spotahome/redis-operator
# Two things this image needs told to it.
#
# It is published for linux/amd64 only, so the platform is named rather than
# left to Docker, which has no arm64 entry to fall back from and refuses to
# start. Naming it costs nothing where the host is already amd64.
#
# It also carries a Go older than the one go.mod asks for, and pins
# GOTOOLCHAIN to local so it will not fetch one. No published tag of it ships a
# Go new enough. Letting it fetch the toolchain go.mod names is what makes the
# generators run at all.
CODEGEN_IMAGE := ghcr.io/slok/kube-code-generator:v0.6.0
CODEGEN_PLATFORM := linux/amd64
PORT := 9710

# The only pin of golangci-lint. The lint job in CI reads it from here, so CI
# and `make lint` always run the same release. Renovate finds it through the
# comment below, which has to stay on the line directly above it.
# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT_IMAGE := golangci/golangci-lint:$(GOLANGCI_LINT_VERSION)

ACTIONLINT_VERSION := 1.7.12

# The chart tests name the helm they run rather than taking whichever one a
# machine happens to have. `helm lint` and `helm template` change what they
# accept and what they render between releases, so a chart that passes against
# one helm can fail against another, and the release that gates a merge should
# be the release a contributor runs.
#
# Renovate finds it through the comment below. Only whitespace may separate the
# two, and the variable name has to end in _VERSION. helm tags its releases with
# a leading v while scripts/helm-tests.sh builds its download URL as
# v${version}, so extractVersion takes the v off.
# renovate: datasource=github-releases depName=helm/helm extractVersion=^v(?<version>.+)$
HELM_VERSION := 3.22.1

# The integration tests run against a kind cluster. KIND_NODE_IMAGE names the node
# image `make test-integration` uses when CI names none, and every CI job names one
# from its matrix. kind publishes a set of node images with each release, and its
# release notes require the digest to identify an image built for that release, so
# this one carries a tag and a digest. The images move when KIND_VERSION moves, and
# the Kind image sync job checks that each one came from the release it names. Keep
# this one to a release the matrix covers, so a local run predicts a CI run.
#
# Renovate finds the kind release through the comment below. Only whitespace may
# separate the two, and the variable name has to end in _VERSION. kind tags its
# releases with a leading v while scripts/integration-tests.sh builds its download
# URL as v${kind_version}, so extractVersion takes the v off.
# renovate: datasource=github-releases depName=kubernetes-sigs/kind extractVersion=^v(?<version>.+)$
KIND_VERSION := 0.33.0
KIND_NODE_IMAGE ?= kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed

# workdir
WORKDIR := /go/src/github.com/spotahome/redis-operator

# CMDs
UNIT_TEST_CMD := go test `go list ./... | grep -v /vendor/` -v
HELM_TEST_CMD := ./scripts/helm-tests.sh
WORKFLOW_LINT_CMD := ./scripts/lint-workflows.sh
GO_GENERATE_CMD := go generate `go list ./... | grep -v /vendor/`
INTEGRATION_TEST_CMD := ./scripts/integration-tests.sh
MOCKS_CMD := go generate ./mocks
# The interactive shell into the dev container, and the only target that wants a
# terminal. Nothing else below allocates one: a generator and a linter read no
# input, and `docker run -it` refuses to start where stdin is not a terminal,
# which is what a CI step and a git hook give it.
DOCKER_RUN_CMD := $(DOCKER) run -it --rm \
	  -v $(PWD):$(WORKDIR) \
	  -u $(UID):$(UID) \
	  --name $(SERVICE_NAME) \
	  -p $(PORT):$(PORT) \
	  $(REPOSITORY)-dev

# The default action of this Makefile is to build the development docker image
.PHONY: default
default: test

.PHONY: ensure-docker
# Test if the dependencies we need to run this Makefile are installed
ensure-docker:
ifndef DOCKER
	@echo "Docker is not available. Please install docker"
	@exit 1
endif

# Build the operator image for local, end-to-end testing purposes
.PHONY: image-local
image-local: ensure-docker
	docker buildx bake \
	  --set build-local.tags="$(IMAGE_NAME):latest" \
	  --set build-local.tags="$(IMAGE_NAME):$(IMAGE_TAG)" \
	  build-local

# Build the development environment docker image
.PHONY: image-dev-tools
image-dev-tools: ensure-docker
	docker buildx bake \
	  --set dev.args.uid=$(UID) \
	  --set dev.tags="$(IMAGE_NAME)-dev:latest" \
	  dev

# Connect to a BASH shell the development docker image
.PHONY: shell
shell: image-dev-tools
	$(DOCKER_RUN_CMD) /bin/bash

# Tag an operator release. The tag name comes from VERSION above. See the
# script for what it refuses.
.PHONY: tag-operator
tag-operator:
	@./scripts/tag-operator.sh

# Tag a chart-only release. The tag name comes from Chart.yaml, where the
# chart's version lives because it moves without the operator. See the script
# for what it refuses.
.PHONY: tag-chart
tag-chart:
	@./scripts/tag-chart.sh

# Tag a release: both the operator and the chart it publishes alongside it.
.PHONY: tag
tag: tag-operator tag-chart

# Run the unit tests. Needs Go on the host, of any patch version: the go
# directive in go.mod names the toolchain, and Go fetches it when the installed
# one is older. CI runs this same target.
.PHONY: test-unit
test-unit:
	$(UNIT_TEST_CMD)

# Run the chart tests. Needs curl and tar on the host: helm is fetched into
# bin/ at the version named above. CI runs this same target.
.PHONY: test-helm
test-helm:
	HELM_VERSION=$(HELM_VERSION) $(HELM_TEST_CMD)

# Run integration tests against a throwaway kind cluster. Needs Go and Docker on
# the host. CI runs this same target.
.PHONY: test-integration
test-integration: ensure-docker
	KIND_VERSION=$(KIND_VERSION) KIND_NODE_IMAGE=$(KIND_NODE_IMAGE) $(INTEGRATION_TEST_CMD)

# Run every test. CI runs these as three jobs rather than this target.
.PHONY: test
test: test-unit test-helm test-integration

# Generate kubernetes client
.PHONY: generate-client
generate-client:
	@echo ">> Generating code for Kubernetes CRD types..."
	docker run --rm \
		--platform $(CODEGEN_PLATFORM) \
		-e GOTOOLCHAIN=auto \
		-v $(PWD):$(WORKDIR) \
		-w $(WORKDIR) \
		$(CODEGEN_IMAGE) \
		--apis-in ./api \
		--go-gen-out ./client/k8s

# Make every version marker agree with VERSION above, after editing it.
#
# The chart's own version is not one of those markers. It is set by hand in
# Chart.yaml, because it moves without the operator. Neither changelog is
# written here either: both want prose. The script lists what it leaves for a
# person when it finishes.
.PHONY: prepare-release
prepare-release:
	@./scripts/prepare-release.sh "$(VERSION)"

# Generate kubernetes Custom Resource Definitions
.PHONY: generate-crd
generate-crd:
	docker run --rm \
		--platform $(CODEGEN_PLATFORM) \
		-e GOTOOLCHAIN=auto \
		-v $(PWD):$(WORKDIR) \
		-w $(WORKDIR) \
		$(CODEGEN_IMAGE) \
		--apis-in ./api \
		--crd-gen-out ./manifests
	cp -f manifests/databases.spotahome.com_redisfailovers.yaml manifests/kustomize/base
	cp -f manifests/databases.spotahome.com_redisfailovers.yaml charts/redisoperator/crds/databases.spotahome.com_redisfailovers.yaml

.PHONY: generate-go
generate-go: image-dev-tools
	docker run --rm \
	  -v $(PWD):$(WORKDIR) \
	  -u $(UID):$(UID) \
	  --name $(SERVICE_NAME) $(REPOSITORY)-dev \
	  /bin/sh -c '$(GO_GENERATE_CMD)'

# Generate testing mocks
.PHONY: generate-mocks
generate-mocks: image-dev-tools
	docker run --rm \
	  -v $(PWD):$(WORKDIR) \
	  -u $(UID):$(UID) \
	  --name $(SERVICE_NAME) \
	  $(REPOSITORY)-dev /bin/sh -c '$(MOCKS_CMD)'

# Run lint
.PHONY: lint
lint:
	docker run --rm \
	  -v $(PWD):$(WORKDIR) \
	  -w $(WORKDIR) \
	  $(GOLANGCI_LINT_IMAGE) \
	  golangci-lint run --fix --timeout=15m

# Lint the GitHub Actions workflow files
.PHONY: lint-workflows
lint-workflows:
	ACTIONLINT_VERSION=$(ACTIONLINT_VERSION) $(WORKFLOW_LINT_CMD)

# Run all code generators
.PHONY: generate
generate: generate-go generate-mocks generate-client generate-crd
