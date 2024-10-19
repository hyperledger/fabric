# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - all (default) - builds all targets and runs all non-integration tests/checks
#   - basic-checks - performs basic checks like license, spelling, trailing spaces and linter
#   - check-deps - check for vendored dependencies that are no longer used
#   - checks - runs all non-integration tests/checks
#   - clean-all - superset of 'clean' that also removes persistent state
#   - clean - cleans the build area
#   - configtxgen - builds a native configtxgen binary
#   - configtxlator - builds a native configtxlator binary
#   - cryptogen - builds a native cryptogen binary
#   - desk-check - runs linters and verify to test changed packages
#   - dist-clean - clean release packages for all target platforms
#   - docker[-clean] - ensures all docker images are available[/cleaned]
#   - docker-list - generates a list of docker images that 'make docker' produces
#   - docker-tag-latest - re-tags the images made by 'make docker' with the :latest tag
#   - docker-tag-stable - re-tags the images made by 'make docker' with the :stable tag
#   - docker-thirdparty - pulls thirdparty images (couchdb, etc)
#   - docs - builds the documentation in html format
#   - gotools - installs go tools like golint
#   - help-docs - generate the command reference docs
#   - integration-test-prereqs - setup prerequisites for integration tests
#   - integration-test - runs the integration tests
#   - ledgerutil - builds a native ledgerutil binary
#   - license - checks go source files for Apache license header
#   - linter - runs all code checks
#   - native - ensures all native binaries are available
#   - orderer - builds a native fabric orderer binary
#   - orderer-docker[-clean] - ensures the orderer container is available[/cleaned]
#   - osnadmin - builds a native fabric osnadmin binary
#   - peer - builds a native fabric peer binary
#   - peer-docker[-clean] - ensures the peer container is available[/cleaned]
#   - profile - runs unit tests for all packages in coverprofile mode (slow)
#   - protos - generate all protobuf artifacts based on .proto files
#   - publish-images - publishes release docker images to nexus3 or docker hub.
#   - release-all - builds release packages for all target platforms
#   - release - builds release packages for the host platform
#   - unit-test-clean - cleans unit test state (particularly from docker)
#   - unit-test - runs the go-test based unit tests
#   - verify - runs unit tests for only the changed package tree

UBUNTU_VER ?= 22.04
FABRIC_VER ?= 3.0.0

# 3rd party image version
# These versions are also set in the runners in ./integration/runners/
COUCHDB_VER ?= 3.3.3

# Disable implicit rules
.SUFFIXES:
MAKEFLAGS += --no-builtin-rules

BUILD_DIR ?= build

EXTRA_VERSION ?= $(shell git rev-parse --short HEAD)
PROJECT_VERSION=$(FABRIC_VER)-snapshot-$(EXTRA_VERSION)

# TWO_DIGIT_VERSION is derived, e.g. "2.0", especially useful as a local tag
# for two digit references to most recent baseos and ccenv patch releases
# TWO_DIGIT_VERSION removes the (optional) semrev 'v' character from the git
# tag triggering a Fabric release.
TWO_DIGIT_VERSION = $(shell echo $(FABRIC_VER) | sed -e  's/^v\(.*\)/\1/' | cut -d '.' -f 1,2)

PKGNAME = github.com/hyperledger/fabric
ARCH=$(shell go env GOARCH)
MARCH=$(shell go env GOOS)-$(shell go env GOARCH)

# defined in common/metadata/metadata.go
METADATA_VAR = Version=$(FABRIC_VER)
METADATA_VAR += CommitSHA=$(EXTRA_VERSION)
METADATA_VAR += BaseDockerLabel=$(BASE_DOCKER_LABEL)
METADATA_VAR += DockerNamespace=$(DOCKER_NS)

GO_VER = 1.23.2
GO_TAGS ?=

RELEASE_EXES = orderer $(TOOLS_EXES)
RELEASE_IMAGES = baseos ccenv orderer peer
RELEASE_PLATFORMS = darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64
TOOLS_EXES = configtxgen configtxlator cryptogen discover ledgerutil osnadmin peer

pkgmap.configtxgen    := $(PKGNAME)/cmd/configtxgen
pkgmap.configtxlator  := $(PKGNAME)/cmd/configtxlator
pkgmap.cryptogen      := $(PKGNAME)/cmd/cryptogen
pkgmap.discover       := $(PKGNAME)/cmd/discover
pkgmap.ledgerutil     := $(PKGNAME)/cmd/ledgerutil
pkgmap.orderer        := $(PKGNAME)/cmd/orderer
pkgmap.osnadmin       := $(PKGNAME)/cmd/osnadmin
pkgmap.peer           := $(PKGNAME)/cmd/peer

.DEFAULT_GOAL := all

include docker-env.mk
include gotools.mk

.PHONY: help
# List all commands with documentation
help: ## List all commands with documentation
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*?## "}; /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: all
all: ## Builds all targets and runs all non-integration tests/checks 
	check-go-version native docker checks

.PHONY: checks
checks: ## Runs basic checks along with unit and integration tests 
	basic-checks unit-test integration-test

.PHONY: basic-checks
basic-checks: check-go-version license spelling references trailing-spaces linter check-help-docs check-metrics-doc filename-spaces check-swagger ## Performs basic checks like license, spelling, trailing spaces and linter 


.PHONY: desk-checks
desk-check: ## Runs linters and verify to test changed packages 
	checks verify

.PHONY: help-docs
help-docs: native ## Generate the command reference docs
	@scripts/help_docs.sh

.PHONY: check-help-docs
check-help-docs: native ## Check for outdated command reference documentation
	@scripts/help_docs.sh check

.PHONY: spelling
spelling: gotool.misspell ## Check for spelling errors
	@scripts/check_spelling.sh

.PHONY: references
references: ## Check for outdated references
	@scripts/check_references.sh

.PHONY: license
license: ## Check for license headers
	@scripts/check_license.sh

.PHONY: trailing-spaces
trailing-spaces: ## Check for trailing spaces
	@scripts/check_trailingspaces.sh

.PHONY: gotools
gotools: gotools-install ## Install go tools like golint

.PHONY: check-go-version
check-go-version: ## Check for the correct go version
	@scripts/check_go_version.sh $(GO_VER)

.PHONY: integration-test
integration-test: integration-test-prereqs ## Runs the integration tests
	./scripts/run-integration-tests.sh $(INTEGRATION_TEST_SUITE)

.PHONY: integration-test-prereqs
integration-test-prereqs: gotool.ginkgo baseos-docker ccenv-docker docker-thirdparty ccaasbuilder ## Setup prerequisites for integration tests

.PHONY: unit-test
unit-test: unit-test-clean docker-thirdparty-couchdb ## Runs the go-test based unit tests
	./scripts/run-unit-tests.sh

.PHONY: unit-tests
unit-tests: unit-test ## Alias for unit-test

# Pull thirdparty docker images based on the latest baseimage release version
# Also pull ccenv-1.4 for compatibility test to ensure pre-2.0 installed chaincodes
# can be built by a peer configured to use the ccenv-1.4 as the builder image.
.PHONY: docker-thirdparty
docker-thirdparty: docker-thirdparty-couchdb ## Pull thirdparty docker images
	docker pull hyperledger/fabric-ccenv:1.4

.PHONY: docker-thirdparty-couchdb
docker-thirdparty-couchdb: ## Pull couchdb docker image
	docker pull couchdb:${COUCHDB_VER}

.PHONY: verify
verify: export JOB_TYPE=VERIFY ## Runs unit tests for only the changed package tree
verify: unit-test # Runs unit tests for only the changed package tree

.PHONY: profile
profile: export JOB_TYPE=PROFILE ## Runs unit tests for all packages in coverprofile mode (slow)
profile: unit-test # Runs unit tests for all packages in coverprofile mode (slow)

.PHONY: linter
linter: check-deps gotool.goimports gotool.gofumpt gotool.staticcheck ## Runs all code checks
	@echo "LINT: Running code checks.."
	./scripts/golinter.sh

.PHONY: check-deps
check-deps: ## Check for vendored dependencies that are no longer used
	@echo "DEP: Checking for dependency issues.."
	./scripts/check_deps.sh

.PHONY: check-metrics-docs
check-metrics-doc: gotool.gendoc ## Check for outdated reference documentation
	@echo "METRICS: Checking for outdated reference documentation.."
	./scripts/metrics_doc.sh check

.PHONY: generate-metrics-docs
generate-metrics-doc: gotool.gendoc ## Generate metrics reference documentation
	@echo "Generating metrics reference documentation..."
	./scripts/metrics_doc.sh generate

.PHONY: check-swagger
check-swagger: gotool.swagger ## Check for outdated swagger
	@echo "SWAGGER: Checking for outdated swagger..."
	./scripts/swagger.sh check

.PHONY: generate-swagger
generate-swagger: gotool.swagger ## Generate swagger
	@echo "Generating swagger..."
	./scripts/swagger.sh generate

.PHONY: protos
protos: gotool.protoc-gen-go ## Generate all protobuf artifacts based on .proto files
	@echo "Compiling non-API protos..."
	./scripts/compile_protos.sh

.PHONY: native
native: $(RELEASE_EXES) ## Ensures all native binaries are available

.PHONY: tools
tools: $(TOOLS_EXES) ## Builds all tools

.PHONY: $(RELEASE_EXES)
$(RELEASE_EXES): %: $(BUILD_DIR)/bin/% ## Builds a native binary

$(BUILD_DIR)/bin/%: GO_LDFLAGS = $(METADATA_VAR:%=-X $(PKGNAME)/common/metadata.%)
$(BUILD_DIR)/bin/%:
	@echo "Building $@"
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) go install -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" -buildvcs=false $(pkgmap.$(@F))
	@touch $@

.PHONY: docker
docker: $(RELEASE_IMAGES:%=%-docker) ccaasbuilder ## Builds all docker images

.PHONY: $(RELEASE_IMAGES:%=%-docker)
$(RELEASE_IMAGES:%=%-docker): %-docker: $(BUILD_DIR)/images/%/$(DUMMY) ## Builds a docker image

$(BUILD_DIR)/images/baseos/$(DUMMY):  BUILD_CONTEXT=images/baseos
$(BUILD_DIR)/images/ccenv/$(DUMMY):   BUILD_CONTEXT=images/ccenv
$(BUILD_DIR)/images/peer/$(DUMMY):    BUILD_ARGS=--build-arg GO_TAGS=${GO_TAGS}
$(BUILD_DIR)/images/orderer/$(DUMMY): BUILD_ARGS=--build-arg GO_TAGS=${GO_TAGS}

$(BUILD_DIR)/images/%/$(DUMMY):
	@echo "Building Docker image $(DOCKER_NS)/fabric-$* with Ubuntu version $(UBUNTU_VER)"
	@mkdir -p $(@D)
	$(DBUILD) -f images/$*/Dockerfile \
		--build-arg GO_VER=$(GO_VER) \
		--build-arg UBUNTU_VER=$(UBUNTU_VER) \
		--build-arg FABRIC_VER=$(FABRIC_VER) \
		--build-arg TARGETARCH=$(ARCH) \
		--build-arg TARGETOS=linux \
		$(BUILD_ARGS) \
		-t $(DOCKER_NS)/fabric-$* \
		-t $(DOCKER_NS)/fabric-$*:$(FABRIC_VER) \
		-t $(DOCKER_NS)/fabric-$*:$(TWO_DIGIT_VERSION) \
		./$(BUILD_CONTEXT)

# builds release packages for the host platform
.PHONY: release
release: check-go-version $(MARCH:%=release/%) ## Builds release packages for the host platform

# builds release packages for all target platforms
.PHONY: release-all
release-all: check-go-version $(RELEASE_PLATFORMS:%=release/%) ## Builds release packages for all target platforms

.PHONY: $(RELEASE_PLATFORMS:%=release/%)
$(RELEASE_PLATFORMS:%=release/%): GO_LDFLAGS = $(METADATA_VAR:%=-X $(PKGNAME)/common/metadata.%)
$(RELEASE_PLATFORMS:%=release/%): release/%: $(foreach exe,$(RELEASE_EXES),release/%/bin/$(exe))
$(RELEASE_PLATFORMS:%=release/%): release/%: ccaasbuilder/%

# explicit targets for all platform executables
$(foreach platform, $(RELEASE_PLATFORMS), $(RELEASE_EXES:%=release/$(platform)/bin/%)):
	$(eval platform = $(patsubst release/%/bin,%,$(@D)))
	$(eval GOOS = $(word 1,$(subst -, ,$(platform))))
	$(eval GOARCH = $(word 2,$(subst -, ,$(platform))))
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" -buildvcs=false $(pkgmap.$(@F))

.PHONY: dist
dist: dist-clean dist/$(MARCH) # Builds release packages for the host platform

.PHONY: dist-all
dist-all: dist-clean $(RELEASE_PLATFORMS:%=dist/%) ## Builds release packages for all target platforms
dist/%: release/% ccaasbuilder
	mkdir -p release/$(@F)/config
	cp -r sampleconfig/*.yaml release/$(@F)/config
	cd release/$(@F) && tar -czvf hyperledger-fabric-$(@F).$(PROJECT_VERSION).tar.gz *

.PHONY: docker-list
docker-list: $(RELEASE_IMAGES:%=%-docker-list) ## Generates a list of docker images that 'make docker' produces
%-docker-list:
	@echo $(DOCKER_NS)/fabric-$*:$(DOCKER_TAG)

.PHONY: docker-clean
docker-clean: $(RELEASE_IMAGES:%=%-docker-clean) ## Ensures all docker images are available
%-docker-clean:
	-@for image in "$$(docker images --quiet --filter=reference='$(DOCKER_NS)/fabric-$*')"; do \
		[ -z "$$image" ] || docker rmi -f $$image; \
	done
	-@rm -rf $(BUILD_DIR)/images/$* || true

.PHONY: docker-tag-latest
docker-tag-latest: $(RELEASE_IMAGES:%=%-docker-tag-latest) ## Re-tags the images made by 'make docker' with the :latest tag
%-docker-tag-latest:
	docker tag $(DOCKER_NS)/fabric-$*:$(DOCKER_TAG) $(DOCKER_NS)/fabric-$*:latest

.PHONY: docker-tag-stable
docker-tag-stable: $(RELEASE_IMAGES:%=%-docker-tag-stable)
%-docker-tag-stable:
	docker tag $(DOCKER_NS)/fabric-$*:$(DOCKER_TAG) $(DOCKER_NS)/fabric-$*:stable

.PHONY: publish-images
publish-images: $(RELEASE_IMAGES:%=%-publish-images)
%-publish-images:
	@docker login $(DOCKER_HUB_USERNAME) $(DOCKER_HUB_PASSWORD)
	@docker push $(DOCKER_NS)/fabric-$*:$(PROJECT_VERSION)

.PHONY: clean
clean: docker-clean unit-test-clean release-clean ## Cleans the build area
	-@rm -rf $(BUILD_DIR)

.PHONY: clean-all
clean-all: clean gotools-clean dist-clean ## Cleans the build area and removes persistent state
	-@rm -rf /var/hyperledger/*
	-@rm -rf docs/build/

.PHONY: dist-clean
dist-clean: ## Clean release packages for all target platforms
	-@for platform in $(RELEASE_PLATFORMS) ""; do \
		[ -z "$$platform" ] || rm -rf release/$${platform}/hyperledger-fabric-$${platform}.$(PROJECT_VERSION).tar.gz; \
	done

.PHONY: release-clean
release-clean: $(RELEASE_PLATFORMS:%=%-release-clean) ## Clean release packages for all target platforms
%-release-clean:
	-@rm -rf release/$*

.PHONY: unit-test-clean
unit-test-clean: 

.PHONY: filename-spaces
spaces: # Check for spaces in file names
	@scripts/check_file_name_spaces.sh

.PHONY: docs
docs: # Builds the documentation in html format
	@docker run --rm -v $$(pwd):/docs python:3.12-slim sh -c 'pip install --no-input tox && cd /docs && tox -e docs'

.PHONY: ccaasbuilder-clean
ccaasbuilder-clean/%:
	$(eval platform = $(patsubst ccaasbuilder/%,%,$@) )
	cd ccaas_builder &&	rm -rf $(strip $(platform))

.PHONY: ccaasbuilder
ccaasbuilder/%: ccaasbuilder-clean
	$(eval platform = $(patsubst ccaasbuilder/%,%,$@) )
	$(eval GOOS = $(word 1,$(subst -, ,$(platform))))
	$(eval GOARCH = $(word 2,$(subst -, ,$(platform))))
	@mkdir -p release/$(strip $(platform))/builders/ccaas/bin
	cd ccaas_builder && go test -v ./cmd/detect && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -o ../release/$(strip $(platform))/builders/ccaas/bin/ ./cmd/detect/
	cd ccaas_builder && go test -v ./cmd/build && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -o ../release/$(strip $(platform))/builders/ccaas/bin/ ./cmd/build/
	cd ccaas_builder && go test -v ./cmd/release && GOOS=$(GOOS) GOARCH=$(GOARCH) go build -buildvcs=false -o ../release/$(strip $(platform))/builders/ccaas/bin/ ./cmd/release/

ccaasbuilder: ccaasbuilder/$(MARCH)

.PHONY: scan
scan: scan-govulncheck ## Run all security scans

.PHONY: scan-govulncheck
scan-govulncheck: gotool.govulncheck ## Run gosec security scan
	govulncheck ./...
