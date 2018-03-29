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
#   - peer - builds a native fabric peer binary
#   - orderer - builds a native fabric orderer binary
#   - release - builds release packages for the host platform
#   - release-all - builds release packages for all target platforms
#   - unit-test - runs the go-test based unit tests
#   - verify - runs unit tests for only the changed package tree
#   - profile - runs unit tests for all packages in coverprofile mode (slow)
#   - test-cmd - generates a "go test" string suitable for manual customization
#   - behave - runs the behave test
#   - behave-deps - ensures pre-requisites are available for running behave manually
#   - gotools - installs go tools like golint
#   - linter - runs all code checks
#   - check-deps - check for vendored dependencies that are no longer used
#   - license - checks go source files for Apache license header
#   - native - ensures all native binaries are available
#   - docker[-clean] - ensures all docker images are available[/cleaned]
#   - peer-docker[-clean] - ensures the peer container is available[/cleaned]
#   - orderer-docker[-clean] - ensures the orderer container is available[/cleaned]
#   - tools-docker[-clean] - ensures the tools container is available[/cleaned]
#   - protos - generate all protobuf artifacts based on .proto files
#   - clean - cleans the build area
#   - clean-all - superset of 'clean' that also removes persistent state
#   - dist-clean - clean release packages for all target platforms
#   - unit-test-clean - cleans unit test state (particularly from docker)
#   - basic-checks - performs basic checks like license, spelling and linter
#   - enable_ci_only_tests - triggers unit-tests in downstream jobs. Applicable only for CI not to
#     use in the local machine.
#   - docker-thirdparty - pulls thirdparty images (kafka,zookeeper,couchdb)

BASE_VERSION = 1.2.0
PREV_VERSION = 1.1.0
CHAINTOOL_RELEASE=1.1.0
BASEIMAGE_RELEASE=0.4.6

# Allow to build as a submodule setting the main project to
# the PROJECT_NAME env variable, for example,
# export PROJECT_NAME=hyperledger/fabric-test
ifeq ($(PROJECT_NAME),true)
PROJECT_NAME = $(PROJECT_NAME)/fabric
else
PROJECT_NAME = hyperledger/fabric
endif
IS_RELEASE = false
EXPERIMENTAL ?= true

BUILD_DIR ?= .build

ifeq ($(EXPERIMENTAL),true)
GO_TAGS += experimental
endif

ifneq ($(IS_RELEASE),true)
EXTRA_VERSION ?= snapshot-$(shell git rev-parse --short HEAD)
PROJECT_VERSION=$(BASE_VERSION)-$(EXTRA_VERSION)
else
PROJECT_VERSION=$(BASE_VERSION)
endif

PKGNAME = github.com/$(PROJECT_NAME)
CGO_FLAGS = CGO_CFLAGS=" "
ARCH=$(shell uname -m)
MARCH=$(shell go env GOOS)-$(shell go env GOARCH)

# defined in common/metadata/metadata.go
METADATA_VAR = Version=$(PROJECT_VERSION)
METADATA_VAR += BaseVersion=$(BASEIMAGE_RELEASE)
METADATA_VAR += BaseDockerLabel=$(BASE_DOCKER_LABEL)
METADATA_VAR += DockerNamespace=$(DOCKER_NS)
METADATA_VAR += BaseDockerNamespace=$(BASE_DOCKER_NS)
METADATA_VAR += Experimental=$(EXPERIMENTAL)

GO_LDFLAGS = $(patsubst %,-X $(PKGNAME)/common/metadata.%,$(METADATA_VAR))

GO_TAGS ?=

CHAINTOOL_URL ?= https://nexus.hyperledger.org/content/repositories/releases/org/hyperledger/fabric/hyperledger-fabric/chaintool-$(CHAINTOOL_RELEASE)/hyperledger-fabric-chaintool-$(CHAINTOOL_RELEASE).jar

export GO_LDFLAGS GO_TAGS

EXECUTABLES = go docker git curl
K := $(foreach exec,$(EXECUTABLES),\
	$(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH: Check dependencies")))

GOSHIM_DEPS = $(shell ./scripts/goListFiles.sh $(PKGNAME)/core/chaincode/shim)
JAVASHIM_DEPS =  $(shell git ls-files core/chaincode/shim/java)
PROTOS = $(shell git ls-files *.proto | grep -v vendor)
# No sense rebuilding when non production code is changed
PROJECT_FILES = $(shell git ls-files  | grep -v ^test | grep -v ^unit-test | \
	grep -v ^bddtests | grep -v ^docs | grep -v _test.go$ | grep -v .md$ | \
	grep -v ^.git | grep -v ^examples | grep -v ^devenv | grep -v .png$ | \
	grep -v ^LICENSE | grep -v ^vendor | grep -v ^gotools )
RELEASE_TEMPLATES = $(shell git ls-files | grep "release/templates")
IMAGES = peer orderer ccenv javaenv buildenv testenv tools
RELEASE_PLATFORMS = windows-amd64 darwin-amd64 linux-amd64 linux-ppc64le linux-s390x
RELEASE_PKGS = configtxgen cryptogen configtxlator peer orderer

pkgmap.cryptogen      := $(PKGNAME)/common/tools/cryptogen
pkgmap.configtxgen    := $(PKGNAME)/common/tools/configtxgen
pkgmap.configtxlator  := $(PKGNAME)/common/tools/configtxlator
pkgmap.peer           := $(PKGNAME)/peer
pkgmap.orderer        := $(PKGNAME)/orderer
pkgmap.block-listener := $(PKGNAME)/examples/events/block-listener

include docker-env.mk

all: native docker checks

checks: basic-checks unit-test

basic-checks: license spelling linter

desk-check: checks verify

# Pull thirdparty docker images based on the latest baseimage release version
.PHONY: docker-thirdparty
docker-thirdparty:
	docker pull $(DOCKER_NS)/fabric-couchdb:$(BASE_DOCKER_TAG)
	docker tag $(DOCKER_NS)/fabric-couchdb:$(BASE_DOCKER_TAG) $(DOCKER_NS)/fabric-couchdb
	docker pull $(DOCKER_NS)/fabric-zookeeper:$(BASE_DOCKER_TAG)
	docker tag $(DOCKER_NS)/fabric-zookeeper:$(BASE_DOCKER_TAG) $(DOCKER_NS)/fabric-zookeeper
	docker pull $(DOCKER_NS)/fabric-kafka:$(BASE_DOCKER_TAG)
	docker tag $(DOCKER_NS)/fabric-kafka:$(BASE_DOCKER_TAG) $(DOCKER_NS)/fabric-kafka

.PHONY: spelling
spelling:
	@scripts/check_spelling.sh

.PHONY: license
license:
	@scripts/check_license.sh

.PHONY: gotools
gotools:
	mkdir -p $(BUILD_DIR)/bin
	cd gotools && $(MAKE) install BINDIR=$(GOPATH)/bin OBJDIR=$(abspath $(BUILD_DIR)/gotools)

.PHONY: gotools-clean
gotools-clean:
	cd gotools && $(MAKE) clean OBJDIR=$(abspath $(BUILD_DIR)/gotools)

# This is a legacy target left to satisfy existing CI scripts
membersrvc-image:
	@echo "membersrvc has been removed from this build"

.PHONY: peer
peer: $(BUILD_DIR)/bin/peer
peer-docker: $(BUILD_DIR)/image/peer/$(DUMMY)

.PHONY: orderer
orderer: $(BUILD_DIR)/bin/orderer
orderer-docker: $(BUILD_DIR)/image/orderer/$(DUMMY)

.PHONY: configtxgen
configtxgen: GO_TAGS+= nopkcs11
configtxgen: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)
configtxgen: $(BUILD_DIR)/bin/configtxgen

configtxlator: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)
configtxlator: $(BUILD_DIR)/bin/configtxlator

cryptogen: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)
cryptogen: $(BUILD_DIR)/bin/cryptogen

tools-docker: $(BUILD_DIR)/image/tools/$(DUMMY)

javaenv: $(BUILD_DIR)/image/javaenv/$(DUMMY)

buildenv: $(BUILD_DIR)/image/buildenv/$(DUMMY)

$(BUILD_DIR)/image/testenv/$(DUMMY): $(BUILD_DIR)/image/buildenv/$(DUMMY)
testenv: $(BUILD_DIR)/image/testenv/$(DUMMY)

unit-test: unit-test-clean peer-docker testenv
	cd unit-test && docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

unit-tests: unit-test

enable_ci_only_tests: testenv
	cd unit-test && docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

verify: unit-test-clean peer-docker testenv
	cd unit-test && JOB_TYPE=VERIFY docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

profile: unit-test-clean peer-docker testenv
	cd unit-test && JOB_TYPE=PROFILE docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

# Generates a string to the terminal suitable for manual augmentation / re-issue, useful for running tests by hand
test-cmd:
	@echo "go test -tags \"$(GO_TAGS)\""

docker: docker-thirdparty $(patsubst %,$(BUILD_DIR)/image/%/$(DUMMY), $(IMAGES))

native: peer orderer configtxgen cryptogen configtxlator

behave-deps: docker peer $(BUILD_DIR)/bin/block-listener configtxgen cryptogen
behave: behave-deps
	@echo "Running behave tests"
	@cd bddtests; behave $(BEHAVE_OPTS)

behave-peer-chaincode: $(BUILD_DIR)/bin/peer peer-docker orderer-docker
	@cd peer/chaincode && behave

linter: check-deps buildenv
	@echo "LINT: Running code checks.."
	@$(DRUN) $(DOCKER_NS)/fabric-buildenv:$(DOCKER_TAG) ./scripts/golinter.sh

check-deps: buildenv
	@echo "DEP: Checking for dependency issues.."
	@$(DRUN) $(DOCKER_NS)/fabric-buildenv:$(DOCKER_TAG) ./scripts/check_deps.sh

$(BUILD_DIR)/%/chaintool: Makefile
	@echo "Installing chaintool"
	@mkdir -p $(@D)
	curl -fL $(CHAINTOOL_URL) > $@
	chmod +x $@

# We (re)build a package within a docker context but persist the $GOPATH/pkg
# directory so that subsequent builds are faster
$(BUILD_DIR)/docker/bin/%: $(PROJECT_FILES)
	$(eval TARGET = ${patsubst $(BUILD_DIR)/docker/bin/%,%,${@}})
	@echo "Building $@"
	@mkdir -p $(BUILD_DIR)/docker/bin $(BUILD_DIR)/docker/$(TARGET)/pkg
	@$(DRUN) \
		-v $(abspath $(BUILD_DIR)/docker/bin):/opt/gopath/bin \
		-v $(abspath $(BUILD_DIR)/docker/$(TARGET)/pkg):/opt/gopath/pkg \
		$(BASE_DOCKER_NS)/fabric-baseimage:$(BASE_DOCKER_TAG) \
		go install -tags "$(GO_TAGS)" -ldflags "$(DOCKER_GO_LDFLAGS)" $(pkgmap.$(@F))
	@touch $@

$(BUILD_DIR)/bin:
	mkdir -p $@

changelog:
	./scripts/changelog.sh v$(PREV_VERSION) v$(BASE_VERSION)

$(BUILD_DIR)/docker/gotools/bin/protoc-gen-go: $(BUILD_DIR)/docker/gotools

$(BUILD_DIR)/docker/gotools: gotools/Makefile
	@mkdir -p $@/bin $@/obj
	@$(DRUN) \
		-v $(abspath $@):/opt/gotools \
		-w /opt/gopath/src/$(PKGNAME)/gotools \
		$(BASE_DOCKER_NS)/fabric-baseimage:$(BASE_DOCKER_TAG) \
		make install BINDIR=/opt/gotools/bin OBJDIR=/opt/gotools/obj

# Both peer and peer-docker depend on ccenv and javaenv (all docker env images it supports).
$(BUILD_DIR)/bin/peer: $(BUILD_DIR)/image/ccenv/$(DUMMY) $(BUILD_DIR)/image/javaenv/$(DUMMY)
$(BUILD_DIR)/image/peer/$(DUMMY): $(BUILD_DIR)/image/ccenv/$(DUMMY) $(BUILD_DIR)/image/javaenv/$(DUMMY)

$(BUILD_DIR)/bin/%: $(PROJECT_FILES)
	@mkdir -p $(@D)
	@echo "$@"
	$(CGO_FLAGS) GOBIN=$(abspath $(@D)) go install -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
	@echo "Binary available as $@"
	@touch $@

# payload definitions'
$(BUILD_DIR)/image/ccenv/payload:      $(BUILD_DIR)/docker/gotools/bin/protoc-gen-go \
				$(BUILD_DIR)/bin/chaintool \
				$(BUILD_DIR)/goshim.tar.bz2
$(BUILD_DIR)/image/javaenv/payload:    $(BUILD_DIR)/javashim.tar.bz2 \
				$(BUILD_DIR)/protos.tar.bz2 \
				settings.gradle
$(BUILD_DIR)/image/peer/payload:       $(BUILD_DIR)/docker/bin/peer \
				$(BUILD_DIR)/sampleconfig.tar.bz2
$(BUILD_DIR)/image/orderer/payload:    $(BUILD_DIR)/docker/bin/orderer \
				$(BUILD_DIR)/sampleconfig.tar.bz2
$(BUILD_DIR)/image/buildenv/payload:   $(BUILD_DIR)/gotools.tar.bz2 \
				$(BUILD_DIR)/docker/gotools/bin/protoc-gen-go
$(BUILD_DIR)/image/testenv/payload:    $(BUILD_DIR)/docker/bin/orderer \
				$(BUILD_DIR)/docker/bin/peer \
				$(BUILD_DIR)/sampleconfig.tar.bz2 \
				images/testenv/install-softhsm2.sh
$(BUILD_DIR)/image/tools/payload:      $(BUILD_DIR)/docker/bin/cryptogen \
	                        $(BUILD_DIR)/docker/bin/configtxgen \
	                        $(BUILD_DIR)/docker/bin/configtxlator \
				$(BUILD_DIR)/docker/bin/peer \
				$(BUILD_DIR)/sampleconfig.tar.bz2

$(BUILD_DIR)/image/%/payload:
	mkdir -p $@
	cp $^ $@

.PRECIOUS: $(BUILD_DIR)/image/%/Dockerfile

$(BUILD_DIR)/image/%/Dockerfile: images/%/Dockerfile.in
	@cat $< \
		| sed -e 's/_BASE_NS_/$(BASE_DOCKER_NS)/g' \
		| sed -e 's/_NS_/$(DOCKER_NS)/g' \
		| sed -e 's/_BASE_TAG_/$(BASE_DOCKER_TAG)/g' \
		| sed -e 's/_TAG_/$(DOCKER_TAG)/g' \
		> $@
	@echo LABEL $(BASE_DOCKER_LABEL).version=$(PROJECT_VERSION) \\>>$@
	@echo "     " $(BASE_DOCKER_LABEL).base.version=$(BASEIMAGE_RELEASE)>>$@

$(BUILD_DIR)/image/%/$(DUMMY): Makefile $(BUILD_DIR)/image/%/payload $(BUILD_DIR)/image/%/Dockerfile
	$(eval TARGET = ${patsubst $(BUILD_DIR)/image/%/$(DUMMY),%,${@}})
	@echo "Building docker $(TARGET)-image"
	$(DBUILD) -t $(DOCKER_NS)/fabric-$(TARGET) $(@D)
	docker tag $(DOCKER_NS)/fabric-$(TARGET) $(DOCKER_NS)/fabric-$(TARGET):$(DOCKER_TAG)
	docker tag $(DOCKER_NS)/fabric-$(TARGET) $(DOCKER_NS)/fabric-$(TARGET):$(ARCH)-latest
	@touch $@

$(BUILD_DIR)/gotools.tar.bz2: $(BUILD_DIR)/docker/gotools
	(cd $</bin && tar -jc *) > $@

$(BUILD_DIR)/goshim.tar.bz2: $(GOSHIM_DEPS)
	@echo "Creating $@"
	@tar -jhc -C $(GOPATH)/src $(patsubst $(GOPATH)/src/%,%,$(GOSHIM_DEPS)) > $@

$(BUILD_DIR)/sampleconfig.tar.bz2: $(shell find sampleconfig -type f)
	(cd sampleconfig && tar -jc *) > $@

$(BUILD_DIR)/javashim.tar.bz2: $(JAVASHIM_DEPS)
$(BUILD_DIR)/protos.tar.bz2: $(PROTOS)

$(BUILD_DIR)/%.tar.bz2:
	@echo "Creating $@"
	@tar -jc $^ > $@

# builds release packages for the host platform
release: $(patsubst %,release/%, $(MARCH))

# builds release packages for all target platforms
release-all: $(patsubst %,release/%, $(RELEASE_PLATFORMS))

release/%: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)

release/windows-amd64: GOOS=windows
release/windows-amd64: GO_TAGS+= nopkcs11
release/windows-amd64: $(patsubst %,release/windows-amd64/bin/%, $(RELEASE_PKGS)) release/windows-amd64/install

release/darwin-amd64: GOOS=darwin
release/darwin-amd64: GO_TAGS+= nopkcs11
release/darwin-amd64: $(patsubst %,release/darwin-amd64/bin/%, $(RELEASE_PKGS)) release/darwin-amd64/install

release/linux-amd64: GOOS=linux
release/linux-amd64: GO_TAGS+= nopkcs11
release/linux-amd64: $(patsubst %,release/linux-amd64/bin/%, $(RELEASE_PKGS)) release/linux-amd64/install

release/%-amd64: DOCKER_ARCH=x86_64
release/%-amd64: GOARCH=amd64
release/linux-%: GOOS=linux

release/linux-ppc64le: GOARCH=ppc64le
release/linux-ppc64le: DOCKER_ARCH=ppc64le
release/linux-ppc64le: GO_TAGS+= nopkcs11
release/linux-ppc64le: $(patsubst %,release/linux-ppc64le/bin/%, $(RELEASE_PKGS)) release/linux-ppc64le/install

release/linux-s390x: GOARCH=s390x
release/linux-s390x: DOCKER_ARCH=s390x
release/linux-s390x: GO_TAGS+= nopkcs11
release/linux-s390x: $(patsubst %,release/linux-s390x/bin/%, $(RELEASE_PKGS)) release/linux-s390x/install

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

release/%/install: $(PROJECT_FILES)
	mkdir -p $(@D)/bin
	@cat $(@D)/../templates/get-docker-images.in \
		| sed -e 's/_NS_/$(DOCKER_NS)/g' \
		| sed -e 's/_ARCH_/$(DOCKER_ARCH)/g' \
		| sed -e 's/_VERSION_/$(PROJECT_VERSION)/g' \
		| sed -e 's/_BASE_DOCKER_TAG_/$(BASE_DOCKER_TAG)/g' \
		> $(@D)/bin/get-docker-images.sh
		@chmod +x $(@D)/bin/get-docker-images.sh

.PHONY: dist
dist: dist-clean dist/$(MARCH)

dist-all: dist-clean $(patsubst %,dist/%, $(RELEASE_PLATFORMS))

dist/%: release/%
	mkdir -p release/$(@F)/config
	cp -r sampleconfig/*.yaml release/$(@F)/config
	cd release/$(@F) && tar -czvf hyperledger-fabric-$(@F).$(PROJECT_VERSION).tar.gz *

.PHONY: protos
protos: buildenv
	@$(DRUN) $(DOCKER_NS)/fabric-buildenv:$(DOCKER_TAG) ./scripts/compile_protos.sh

%-docker-clean:
	$(eval TARGET = ${patsubst %-docker-clean,%,${@}})
	-docker images -q $(DOCKER_NS)/fabric-$(TARGET) | xargs -I '{}' docker rmi -f '{}'
	-@rm -rf $(BUILD_DIR)/image/$(TARGET)

docker-clean: $(patsubst %,%-docker-clean, $(IMAGES))

.PHONY: clean
clean: docker-clean unit-test-clean release-clean
	-@rm -rf $(BUILD_DIR)

.PHONY: clean-all
clean-all: clean gotools-clean dist-clean
	-@rm -rf /var/hyperledger/*

.PHONY: dist-clean
dist-clean:
	-@rm -rf release/windows-amd64/hyperledger-fabric-windows-amd64.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/darwin-amd64/hyperledger-fabric-darwin-amd64.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/linux-amd64/hyperledger-fabric-linux-amd64.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/linux-ppc64le/hyperledger-fabric-linux-ppc64le.$(PROJECT_VERSION).tar.gz
	-@rm -rf release/linux-s390x/hyperledger-fabric-linux-s390x.$(PROJECT_VERSION).tar.gz

%-release-clean:
	$(eval TARGET = ${patsubst %-release-clean,%,${@}})
	-@rm -rf release/$(TARGET)

release-clean: $(patsubst %,%-release-clean, $(RELEASE_PLATFORMS))

.PHONY: unit-test-clean
unit-test-clean:
	cd unit-test && docker-compose down
