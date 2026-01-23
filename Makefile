VERSION := v4.3.2

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
CODEGEN_IMAGE := ghcr.io/slok/kube-code-generator:v0.6.0
PORT := 9710

# workdir
WORKDIR := /go/src/github.com/spotahome/redis-operator

# CMDs
UNIT_TEST_CMD := go test `go list ./... | grep -v /vendor/` -v
HELM_TEST_CMD := ./scripts/helm-tests.sh
GO_GENERATE_CMD := go generate `go list ./... | grep -v /vendor/`
GO_INTEGRATION_TEST_CMD := go test `go list ./... | grep test/integration` -v -tags='integration'
MOCKS_CMD := go generate ./mocks
DOCKER_RUN_CMD :=	$(DOCKER) run -ti --rm \
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

# Create a git tag using the VERSION
.PHONY: tag
tag:
	git tag $(VERSION)

# Run unit tests in the development docker container (DEV)
.PHONY: test-unit
test-unit: image-dev-tools
	$(DOCKER_RUN_CMD) /bin/sh -c '$(UNIT_TEST_CMD)'

# Run helm tests in the development docker container (DEV)
.PHONY: test-helm
test-helm:
	$(DOCKER_RUN_CMD) $(HELM_TEST_CMD)

# Run all (DEV) tests
.PHONY: test
test: test-unit test-helm

# Run unit tests on the host (CI)
.PHONY: test-unit-ci
test-unit-ci:
	$(UNIT_TEST_CMD)

# Run integration tests on the host (CI)
.PHONY: test-integration-ci
test-integration-ci:
	$(GO_INTEGRATION_TEST_CMD)

# Run helm tests on the host (CI)
.PHONY: test-helm-ci
test-helm-ci:
	$(HELM_TEST_CMD)

# Generate kubernetes client
.PHONY: generate-client
generate-client:
	@echo ">> Generating code for Kubernetes CRD types..."
	docker run --rm -it \
		-v $(PWD):$(WORKDIR) \
		-w $(PWD):$(WORKDIR) \
		$(CODEGEN_IMAGE) \
		--apis-in $(WORKDIR)/api \
		--go-gen-out $(WORKDIR)/client/k8s

# Generate kubernetes Custom Resource Definitions
.PHONY: generate-crd
generate-crd:
	docker run --rm -it \
		-v $(PWD):$(WORKDIR) \
		-w $(PWD):$(WORKDIR) \
		$(CODEGEN_IMAGE) \
		--apis-in $(WORKDIR)/api \
		--crd-gen-out $(WORKDIR)/manifests
	cp -f manifests/databases.spotahome.com_redisfailovers.yaml manifests/kustomize/base

.PHONY: generate-go
generate-go: image-dev-tools
	docker run -ti --rm \
	  -v $(PWD):$(WORKDIR) \
	  -u $(UID):$(UID) \
	  --name $(SERVICE_NAME) $(REPOSITORY)-dev \
	  /bin/sh -c '$(GO_GENERATE_CMD)'

# Generate testing mocks
.PHONY: generate-mocks
generate-mocks: image-dev-tools
	docker run -ti --rm \
	  -v $(PWD):$(WORKDIR) \
	  -u $(UID):$(UID) \
	  --name $(SERVICE_NAME) \
	  $(REPOSITORY)-dev /bin/sh -c '$(MOCKS_CMD)'

# Run all code generators
.PHONY: generate
generate: generate-go generate-mocks generate-client generate-crd
