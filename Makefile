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
PREV_VERSION = 2.0.0-alpha
BASEIMAGE_RELEASE = 0.4.15

# 3rd party image version
COUCHDB_VER ?= 2.3

# Disable impliicit rules
.SUFFIXES:
MAKEFLAGS += --no-builtin-rules

BUILD_DIR ?= build
NEXUS_REPO = nexus3.hyperledger.org:10001/hyperledger

EXTRA_VERSION ?= $(shell git rev-parse --short HEAD)
PROJECT_VERSION=$(BASE_VERSION)-snapshot-$(EXTRA_VERSION)

PKGNAME = github.com/hyperledger/fabric
CGO_FLAGS = CGO_CFLAGS=" "
ARCH=$(shell go env GOARCH)
MARCH=$(shell go env GOOS)-$(shell go env GOARCH)

# defined in common/metadata/metadata.go
METADATA_VAR = Version=$(BASE_VERSION)
METADATA_VAR += CommitSHA=$(EXTRA_VERSION)
METADATA_VAR += BaseVersion=$(BASEIMAGE_RELEASE)
METADATA_VAR += BaseDockerLabel=$(BASE_DOCKER_LABEL)
METADATA_VAR += DockerNamespace=$(DOCKER_NS)
METADATA_VAR += BaseDockerNamespace=$(BASE_DOCKER_NS)

# GO_VER = $(shell grep "GO_VER" ci.properties |cut -d'=' -f2-)
# temporary override until CI support 1.12.7
GO_VER = 1.12.7
GO_LDFLAGS = $(patsubst %,-X $(PKGNAME)/common/metadata.%,$(METADATA_VAR))

GO_TAGS ?=

# No sense rebuilding when non production code is changed
PROJECT_FILES = $(shell git ls-files  | grep -Ev '^integration/|^vagrant/|.png$|^LICENSE|^vendor/')
RELEASE_IMAGES = baseos ccenv orderer peer tools
RELEASE_PLATFORMS = darwin-amd64 linux-amd64 linux-ppc64le linux-s390x windows-amd64
RELEASE_PKGS = configtxgen cryptogen idemixgen discover configtxlator peer orderer

pkgmap.configtxgen    := $(PKGNAME)/cmd/configtxgen
pkgmap.configtxlator  := $(PKGNAME)/cmd/configtxlator
pkgmap.cryptogen      := $(PKGNAME)/cmd/cryptogen
pkgmap.discover       := $(PKGNAME)/cmd/discover
pkgmap.idemixgen      := $(PKGNAME)/cmd/idemixgen
pkgmap.orderer        := $(PKGNAME)/cmd/orderer
pkgmap.peer           := $(PKGNAME)/cmd/peer

include docker-env.mk

all: check-go-version native docker checks

checks: basic-checks unit-test integration-test

basic-checks: check-go-version license spelling references trailing-spaces linter check-metrics-doc

desk-check: checks verify

help-docs: native
	@scripts/generateHelpDocs.sh

# Pull thirdparty docker images based on the latest baseimage release version
.PHONY: docker-thirdparty
docker-thirdparty:
	docker pull couchdb:${COUCHDB_VER}
	docker pull $(BASE_DOCKER_NS)/fabric-zookeeper:$(BASE_DOCKER_TAG)
	docker tag $(BASE_DOCKER_NS)/fabric-zookeeper:$(BASE_DOCKER_TAG) $(DOCKER_NS)/fabric-zookeeper
	docker pull $(BASE_DOCKER_NS)/fabric-kafka:$(BASE_DOCKER_TAG)
	docker tag $(BASE_DOCKER_NS)/fabric-kafka:$(BASE_DOCKER_TAG) $(DOCKER_NS)/fabric-kafka

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

include gotools.mk

.PHONY: gotools
gotools: gotools-install

tools-docker: $(BUILD_DIR)/images/tools/$(DUMMY)

.PHONY: baseos
baseos: $(BUILD_DIR)/images/baseos/$(DUMMY)

.PHONY: ccenv
ccenv: $(BUILD_DIR)/images/ccenv/$(DUMMY)

.PHONY: peer
peer: $(BUILD_DIR)/bin/peer
peer-docker: $(BUILD_DIR)/images/peer/$(DUMMY)

.PHONY: orderer
orderer: $(BUILD_DIR)/bin/orderer
orderer-docker: $(BUILD_DIR)/images/orderer/$(DUMMY)

.PHONY: configtxgen
configtxgen: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.CommitSHA=$(EXTRA_VERSION)
configtxgen: $(BUILD_DIR)/bin/configtxgen

.PHONY: configtxlator
configtxlator: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.CommitSHA=$(EXTRA_VERSION)
configtxlator: $(BUILD_DIR)/bin/configtxlator

.PHONY: cryptogen
cryptogen: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.CommitSHA=$(EXTRA_VERSION)
cryptogen: $(BUILD_DIR)/bin/cryptogen

.PHONY: idemixgen
idemixgen: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.CommitSHA=$(EXTRA_VERSION)
idemixgen: $(BUILD_DIR)/bin/idemixgen

.PHONY: discover
discover: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)
discover: $(BUILD_DIR)/bin/discover

.PHONY: check-go-version
check-go-version:
	@scripts/check_go_version.sh

.PHONY: integration-test
integration-test: gotool.ginkgo ccenv baseos docker-thirdparty
	./scripts/run-integration-tests.sh

unit-test: unit-test-clean docker-thirdparty ccenv baseos
	./scripts/run-unit-tests.sh

unit-tests: unit-test

verify: export JOB_TYPE=VERIFY
verify: unit-test

profile: export JOB_TYPE=PROFILE
profile: unit-test

docker: $(patsubst %,$(BUILD_DIR)/images/%/$(DUMMY), $(RELEASE_IMAGES))

native: $(RELEASE_PKGS)

linter: check-deps gotools
	@echo "LINT: Running code checks.."
	./scripts/golinter.sh

check-deps: gotools
	@echo "DEP: Checking for dependency issues.."
	./scripts/check_deps.sh

check-metrics-doc: gotools
	@echo "METRICS: Checking for outdated reference documentation.."
	./scripts/metrics_doc.sh check

generate-metrics-doc: gotools
	@echo "Generating metrics reference documentation..."
	./scripts/metrics_doc.sh generate

.PHONY: protos
protos: gotools
	@echo "Compiling non-API protos..."
	./scripts/compile_protos.sh

changelog:
	./scripts/changelog.sh v$(PREV_VERSION) v$(BASE_VERSION)

$(BUILD_DIR)/bin:
	@mkdir -p $@

$(BUILD_DIR)/bin/%: $(PROJECT_FILES)
	@mkdir -p $(@D)
	@echo "$@"
	$(CGO_FLAGS) GOBIN=$(abspath $(@D)) go install -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
	@echo "Binary available as $@"
	@touch $@

$(BUILD_DIR)/images/peer/$(DUMMY): BUILD_ARGS=--build-arg GO_TAGS=${GO_TAGS}

$(BUILD_DIR)/images/orderer/$(DUMMY): BUILD_ARGS=--build-arg GO_TAGS=${GO_TAGS}

$(BUILD_DIR)/images/%/$(DUMMY):
	@mkdir -p $(@D)
	$(eval TARGET = ${patsubst $(BUILD_DIR)/images/%/$(DUMMY),%,${@}})
	@echo "Docker:  building $(TARGET) image"
	$(DBUILD) -f images/$(TARGET)/Dockerfile \
		--build-arg GO_VER=${GO_VER} \
		--build-arg ALPINE_VER=${ALPINE_VER} \
		${BUILD_ARGS} \
		-t $(DOCKER_NS)/fabric-$(TARGET) .
	docker tag $(DOCKER_NS)/fabric-$(TARGET) $(DOCKER_NS)/fabric-$(TARGET):$(BASE_VERSION)
	docker tag $(DOCKER_NS)/fabric-$(TARGET) $(DOCKER_NS)/fabric-$(TARGET):$(DOCKER_TAG)
	@touch $@

# builds release packages for the host platform
release: check-go-version $(patsubst %,release/%, $(MARCH))

# builds release packages for all target platforms
release-all: check-go-version $(patsubst %,release/%, $(RELEASE_PLATFORMS))

release/%: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.CommitSHA=$(EXTRA_VERSION)

release/windows-amd64: GOOS=windows
release/windows-amd64: $(patsubst %,release/windows-amd64/bin/%, $(RELEASE_PKGS))

release/darwin-amd64: GOOS=darwin
release/darwin-amd64: $(patsubst %,release/darwin-amd64/bin/%, $(RELEASE_PKGS))

release/linux-amd64: GOOS=linux
release/linux-amd64: $(patsubst %,release/linux-amd64/bin/%, $(RELEASE_PKGS))

release/%-amd64: GOARCH=amd64
release/linux-%: GOOS=linux

release/linux-s390x: GOARCH=s390x
release/linux-s390x: $(patsubst %,release/linux-s390x/bin/%, $(RELEASE_PKGS))

release/linux-ppc64le: GOARCH=ppc64le
release/linux-ppc64le: $(patsubst %,release/linux-ppc64le/bin/%, $(RELEASE_PKGS))

release/%/bin/configtxlator: $(PROJECT_FILES)
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

release/%/bin/configtxgen: $(PROJECT_FILES)
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

release/%/bin/cryptogen: $(PROJECT_FILES)
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

release/%/bin/idemixgen: $(PROJECT_FILES)
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

release/%/bin/discover: $(PROJECT_FILES)
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

release/%/bin/orderer: GO_LDFLAGS = $(patsubst %,-X $(PKGNAME)/common/metadata.%,$(METADATA_VAR))
release/%/bin/orderer: $(PROJECT_FILES)
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

release/%/bin/peer: GO_LDFLAGS = $(patsubst %,-X $(PKGNAME)/common/metadata.%,$(METADATA_VAR))
release/%/bin/peer: $(PROJECT_FILES)
	@echo "Building $@ for $(GOOS)-$(GOARCH)"
	mkdir -p $(@D)
	$(CGO_FLAGS) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(abspath $@) -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))

.PHONY: dist
dist: dist-clean dist/$(MARCH)

dist-all: dist-clean $(patsubst %,dist/%, $(RELEASE_PLATFORMS))

dist/%: release/%
	mkdir -p release/$(@F)/config
	cp -r sampleconfig/*.yaml release/$(@F)/config
	cd release/$(@F) && tar -czvf hyperledger-fabric-$(@F).$(PROJECT_VERSION).tar.gz *

%-docker-list:
	$(eval TARGET = ${patsubst %-docker-list,%,${@}})
	@echo $(DOCKER_NS)/fabric-$(TARGET):$(DOCKER_TAG)

docker-list: $(patsubst %,%-docker-list, $(RELEASE_IMAGES))

%-docker-clean:
	$(eval TARGET = ${patsubst %-docker-clean,%,${@}})
	$(eval DOCKER_IMAGES = $(shell docker images --quiet --filter=reference='$(DOCKER_NS)/fabric-$(TARGET):$(ARCH)-$(BASE_VERSION)$(if $(EXTRA_VERSION),-snapshot-*,)'))
	[ -n "$(DOCKER_IMAGES)" ] && docker rmi -f $(DOCKER_IMAGES) || true
	-@rm -rf $(BUILD_DIR)/images/$(TARGET) ||:

docker-clean: $(patsubst %,%-docker-clean, $(RELEASE_IMAGES))

docker-tag-latest: $(IMAGES:%=%-docker-tag-latest)

%-docker-tag-latest:
	$(eval TARGET = ${patsubst %-docker-tag-latest,%,${@}})
	docker tag $(DOCKER_NS)/fabric-$(TARGET):$(DOCKER_TAG) $(DOCKER_NS)/fabric-$(TARGET):latest

docker-tag-stable: $(IMAGES:%=%-docker-tag-stable)

%-docker-tag-stable:
	$(eval TARGET = ${patsubst %-docker-tag-stable,%,${@}})
	docker tag $(DOCKER_NS)/fabric-$(TARGET):$(DOCKER_TAG) $(DOCKER_NS)/fabric-$(TARGET):stable

publish-images: $(RELEASE_IMAGES:%=%-publish-images) ## Build and publish docker images

%-publish-images:
	$(eval TARGET = ${patsubst %-publish-images,%,${@}})
	@docker login $(DOCKER_HUB_USERNAME) $(DOCKER_HUB_PASSWORD)
	@docker push $(DOCKER_NS)/fabric-$(TARGET):$(PROJECT_VERSION)

.PHONY: clean
clean: docker-clean unit-test-clean release-clean
	-@rm -rf $(BUILD_DIR)

.PHONY: clean-all
clean-all: clean gotools-clean dist-clean
	-@rm -rf /var/hyperledger/*
	-@rm -rf docs/build/

.PHONY: dist-clean
dist-clean:
	-@rm -rf release/windows-amd64/hyperledger-fabric-windows-amd64.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/darwin-amd64/hyperledger-fabric-darwin-amd64.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/linux-amd64/hyperledger-fabric-linux-amd64.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/linux-s390x/hyperledger-fabric-linux-s390x.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/linux-ppc64le/hyperledger-fabric-linux-ppc64le.$(PROJECT_VERSION).tar.gz

%-release-clean:
	$(eval TARGET = ${patsubst %-release-clean,%,${@}})
	-@rm -rf release/$(TARGET)

release-clean: $(patsubst %,%-release-clean, $(RELEASE_PLATFORMS))

.PHONY: unit-test-clean
unit-test-clean:
