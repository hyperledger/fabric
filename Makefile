# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - all (default) - builds all targets and runs all non-integration tests/checks
#   - checks - runs all non-integration tests/checks
#   - desk-check - runs linters and verify to test changed packages
#   - configtxgen - builds a native configtxgen binary
#   - configtxlator - builds a native configtxlator binary
#   - cryptogen  -  builds a native cryptogen binary
#   - idemixgen  -  builds a native idemixgen binary
#   - peer - builds a native fabric peer binary
#   - orderer - builds a native fabric orderer binary
#   - release - builds release packages for the host platform
#   - release-all - builds release packages for all target platforms
#   - publish-images - publishes release docker images to nexus3 or docker hub.
#   - unit-test - runs the go-test based unit tests
#   - verify - runs unit tests for only the changed package tree
#   - profile - runs unit tests for all packages in coverprofile mode (slow)
#   - gotools - installs go tools like golint
#   - linter - runs all code checks
#   - check-deps - check for vendored dependencies that are no longer used
#   - license - checks go source files for Apache license header
#   - native - ensures all native binaries are available
#   - docker[-clean] - ensures all docker images are available[/cleaned]
#   - docker-list - generates a list of docker images that 'make docker' produces
#   - peer-docker[-clean] - ensures the peer container is available[/cleaned]
#   - orderer-docker[-clean] - ensures the orderer container is available[/cleaned]
#   - tools-docker[-clean] - ensures the tools container is available[/cleaned]
#   - protos - generate all protobuf artifacts based on .proto files
#   - clean - cleans the build area
#   - clean-all - superset of 'clean' that also removes persistent state
#   - dist-clean - clean release packages for all target platforms
#   - unit-test-clean - cleans unit test state (particularly from docker)
#   - basic-checks - performs basic checks like license, spelling, trailing spaces and linter
#   - docker-thirdparty - pulls thirdparty images (kafka,zookeeper,couchdb)
#   - docker-tag-latest - re-tags the images made by 'make docker' with the :latest tag
#   - docker-tag-stable - re-tags the images made by 'make docker' with the :stable tag
#   - help-docs - generate the command reference docs

ALPINE_VER ?= 3.10
BASE_VERSION = 2.0.0
PREV_VERSION = 2.0.0-beta

# BASEIMAGE_RELEASE should be removed now
BASEIMAGE_RELEASE = 0.4.18

# 3rd party image version
# These versions are also set in the runners in ./integration/runners/
COUCHDB_VER ?= 2.3
KAFKA_VER ?= 5.3.1
ZOOKEEPER_VER ?= 5.3.1

# Disable implicit rules
.SUFFIXES:
MAKEFLAGS += --no-builtin-rules

BUILD_DIR ?= build

EXTRA_VERSION ?= $(shell git rev-parse --short HEAD)
PROJECT_VERSION=$(BASE_VERSION)-snapshot-$(EXTRA_VERSION)

#TWO_DIGIT_VERSION is derived, e.g. "2.0", especially useful as a local tag for two digit references to most recent baseos and ccenv patch releases
TWO_DIGIT_VERSION = $(shell echo $(BASE_VERSION) | cut -d'.' -f1,2)

PKGNAME = github.com/hyperledger/fabric
ARCH=$(shell go env GOARCH)
MARCH=$(shell go env GOOS)-$(shell go env GOARCH)

# defined in common/metadata/metadata.go
METADATA_VAR = Version=$(BASE_VERSION)
METADATA_VAR += CommitSHA=$(EXTRA_VERSION)
METADATA_VAR += BaseDockerLabel=$(BASE_DOCKER_LABEL)
METADATA_VAR += DockerNamespace=$(DOCKER_NS)
METADATA_VAR += BaseDockerNamespace=$(BASE_DOCKER_NS)

GO_VER = 1.13.4
GO_TAGS ?=

RELEASE_EXES = orderer $(TOOLS_EXES)
RELEASE_IMAGES = baseos ccenv orderer peer tools
RELEASE_PLATFORMS = darwin-amd64 linux-amd64 linux-ppc64le linux-s390x windows-amd64
TOOLS_EXES = configtxgen configtxlator cryptogen discover idemixgen peer

pkgmap.configtxgen    := $(PKGNAME)/cmd/configtxgen
pkgmap.configtxlator  := $(PKGNAME)/cmd/configtxlator
pkgmap.cryptogen      := $(PKGNAME)/cmd/cryptogen
pkgmap.discover       := $(PKGNAME)/cmd/discover
pkgmap.idemixgen      := $(PKGNAME)/cmd/idemixgen
pkgmap.orderer        := $(PKGNAME)/cmd/orderer
pkgmap.peer           := $(PKGNAME)/cmd/peer

.DEFAULT_GOAL := all

include docker-env.mk
include gotools.mk

.PHONY: all
all: check-go-version native docker checks

.PHONY: checks
checks: basic-checks unit-test integration-test

.PHONY: basic-checks
basic-checks: check-go-version license spelling references trailing-spaces linter check-metrics-doc filename-spaces

.PHONY: desk-checks
desk-check: checks verify

.PHONY: help-docs
help-docs: native
	@scripts/generateHelpDocs.sh

.PHONY: spelling
spelling:
	@scripts/check_spelling.sh

.PHONY: references
references:
	@scripts/check_references.sh

.PHONY: license
license:
	@scripts/check_license.sh

.PHONY: trailing-spaces
trailing-spaces:
	@scripts/check_trailingspaces.sh

.PHONY: gotools
gotools: gotools-install

.PHONY: check-go-version
check-go-version:
	@scripts/check_go_version.sh $(GO_VER)

.PHONY: integration-test
integration-test: gotool.ginkgo ccenv-docker baseos-docker docker-thirdparty
	./scripts/run-integration-tests.sh

.PHONY: unit-test
unit-test: unit-test-clean docker-thirdparty ccenv-docker baseos-docker
	./scripts/run-unit-tests.sh

.PHONY: unit-tests
unit-tests: unit-test

# Pull thirdparty docker images based on the latest baseimage release version
# Also pull ccenv-1.4 for compatibility test to ensure pre-2.0 installed chaincodes
# can be built by a peer configured to use the ccenv-1.4 as the builder image.
.PHONY: docker-thirdparty
docker-thirdparty:
	docker pull couchdb:${COUCHDB_VER}
	docker pull confluentinc/cp-zookeeper:${ZOOKEEPER_VER}
	docker pull confluentinc/cp-kafka:${KAFKA_VER}
	docker pull hyperledger/fabric-ccenv:1.4

.PHONY: verify
verify: export JOB_TYPE=VERIFY
verify: unit-test

.PHONY: profile
profile: export JOB_TYPE=PROFILE
profile: unit-test

.PHONY: linter
linter: check-deps gotools
	@echo "LINT: Running code checks.."
	./scripts/golinter.sh

.PHONY: check-deps
check-deps: gotools
	@echo "DEP: Checking for dependency issues.."
	./scripts/check_deps.sh

.PHONY: check-metrics-docs
check-metrics-doc: gotools
	@echo "METRICS: Checking for outdated reference documentation.."
	./scripts/metrics_doc.sh check

.PHONY: generate-metrics-docs
generate-metrics-doc: gotools
	@echo "Generating metrics reference documentation..."
	./scripts/metrics_doc.sh generate

.PHONY: protos
protos: gotools
	@echo "Compiling non-API protos..."
	./scripts/compile_protos.sh

.PHONY: changelog
changelog:
	./scripts/changelog.sh v$(PREV_VERSION) v$(BASE_VERSION)

.PHONY: native
native: $(RELEASE_EXES)

.PHONY: $(RELEASE_EXES)
$(RELEASE_EXES): %: $(BUILD_DIR)/bin/%

$(BUILD_DIR)/bin/%: GO_LDFLAGS = $(METADATA_VAR:%=-X $(PKGNAME)/common/metadata.%)
$(BUILD_DIR)/bin/%:
	@echo "Building $@"
	@mkdir -p $(@D)
	GOBIN=$(abspath $(@D)) go install -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
	@touch $@

.PHONY: docker
docker: $(RELEASE_IMAGES:%=%-docker)

.PHONY: $(RELEASE_IMAGES:%=%-docker)
$(RELEASE_IMAGES:%=%-docker): %-docker: $(BUILD_DIR)/images/%/$(DUMMY)

$(BUILD_DIR)/images/ccenv/$(DUMMY):   BUILD_CONTEXT=images/ccenv
$(BUILD_DIR)/images/baseos/$(DUMMY):  BUILD_CONTEXT=images/baseos
$(BUILD_DIR)/images/peer/$(DUMMY):    BUILD_ARGS=--build-arg GO_TAGS=${GO_TAGS}
$(BUILD_DIR)/images/orderer/$(DUMMY): BUILD_ARGS=--build-arg GO_TAGS=${GO_TAGS}

$(BUILD_DIR)/images/%/$(DUMMY):
	@echo "Building Docker image $(DOCKER_NS)/fabric-$*"
	@mkdir -p $(@D)
	$(DBUILD) -f images/$*/Dockerfile \
		--build-arg GO_VER=$(GO_VER) \
		--build-arg ALPINE_VER=$(ALPINE_VER) \
		$(BUILD_ARGS) \
		-t $(DOCKER_NS)/fabric-$* ./$(BUILD_CONTEXT)
	docker tag $(DOCKER_NS)/fabric-$* $(DOCKER_NS)/fabric-$*:$(BASE_VERSION)
	docker tag $(DOCKER_NS)/fabric-$* $(DOCKER_NS)/fabric-$*:$(TWO_DIGIT_VERSION)
	docker tag $(DOCKER_NS)/fabric-$* $(DOCKER_NS)/fabric-$*:$(DOCKER_TAG)
	@touch $@

# builds release packages for the host platform
.PHONY: release
release: check-go-version $(MARCH:%=release/%)

# builds release packages for all target platforms
.PHONY: release-all
release-all: check-go-version $(RELEASE_PLATFORMS:%=release/%)

.PHONY: $(RELEASE_PLATFORMS:%=release/%)
$(RELEASE_PLATFORMS:%=release/%): GO_LDFLAGS = $(METADATA_VAR:%=-X $(PKGNAME)/common/metadata.%)
$(RELEASE_PLATFORMS:%=release/%): release/%: $(foreach exe,$(RELEASE_EXES),release/%/bin/$(exe))

# explicit targets for all platform executables
$(foreach platform, $(RELEASE_PLATFORMS), $(RELEASE_EXES:%=release/$(platform)/bin/%)):
	$(eval platform = $(patsubst release/%/bin,%,$(@D)))
	$(eval GOOS = $(word 1,$(subst -, ,$(platform))))
	$(eval GOARCH = $(word 2,$(subst -, ,$(platform))))
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $@ -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

.PHONY: dist
dist: dist-clean dist/$(MARCH)

.PHONY: dist-all
dist-all: dist-clean $(RELEASE_PLATFORMS:%=dist/%)
dist/%: release/%
	mkdir -p release/$(@F)/config
	cp -r sampleconfig/*.yaml release/$(@F)/config
	cd release/$(@F) && tar -czvf hyperledger-fabric-$(@F).$(PROJECT_VERSION).tar.gz *

.PHONY: docker-list
docker-list: $(RELEASE_IMAGES:%=%-docker-list)
%-docker-list:
	@echo $(DOCKER_NS)/fabric-$*:$(DOCKER_TAG)

.PHONY: docker-clean
docker-clean: $(RELEASE_IMAGES:%=%-docker-clean)
%-docker-clean:
	-@for image in "$$(docker images --quiet --filter=reference='$(DOCKER_NS)/fabric-$*:$(DOCKER_TAG)')"; do \
		[ -z "$$image" ] || docker rmi -f $$image; \
	done
	-@rm -rf $(BUILD_DIR)/images/$* || true

.PHONY: docker-tag-latest
docker-tag-latest: $(RELEASE_IMAGES:%=%-docker-tag-latest)
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
clean: docker-clean unit-test-clean release-clean
	-@rm -rf $(BUILD_DIR)

.PHONY: clean-all
clean-all: clean gotools-clean dist-clean
	-@rm -rf /var/hyperledger/*
	-@rm -rf docs/build/

.PHONY: dist-clean
dist-clean:
	-@for platform in $(RELEASE_PLATFORMS) ""; do \
		[ -z "$$platform" ] || rm -rf release/$${platform}/hyperledger-fabric-$${platform}.$(PROJECT_VERSION).tar.gz; \
	done

.PHONY: release-clean
release-clean: $(RELEASE_PLATFORMS:%=%-release-clean)
%-release-clean:
	-@rm -rf release/$*

.PHONY: unit-test-clean
unit-test-clean:

.PHONY: filename-spaces
spaces:
	@scripts/check_file_name_spaces.sh
