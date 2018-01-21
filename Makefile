# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - all (default) - builds all targets and runs all tests/checks
#   - checks - runs all tests/checks
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
#   - test-cmd - generates a "go test" string suitable for manual customization
#   - behave - runs the behave test
#   - behave-deps - ensures pre-requisites are available for running behave manually
#   - gotools - installs go tools like golint
#   - linter - runs all code checks
#   - license - checks go sourrce files for Apache license header
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

PROJECT_NAME   = hyperledger/fabric
BASE_VERSION = 1.0.6
PREV_VERSION = 1.0.5
IS_RELEASE = false

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
CHAINTOOL_RELEASE=1.0.0
BASEIMAGE_RELEASE=$(shell cat ./.baseimage-release)

# defined in common/metadata/metadata.go
METADATA_VAR = Version=$(PROJECT_VERSION)
METADATA_VAR += BaseVersion=$(BASEIMAGE_RELEASE)
METADATA_VAR += BaseDockerLabel=$(BASE_DOCKER_LABEL)
METADATA_VAR += DockerNamespace=$(DOCKER_NS)
METADATA_VAR += BaseDockerNamespace=$(BASE_DOCKER_NS)

GO_LDFLAGS = $(patsubst %,-X $(PKGNAME)/common/metadata.%,$(METADATA_VAR))

GO_TAGS ?=

CHAINTOOL_URL ?= https://nexus.hyperledger.org/content/repositories/releases/org/hyperledger/fabric/hyperledger-fabric/chaintool-$(CHAINTOOL_RELEASE)/hyperledger-fabric-chaintool-$(CHAINTOOL_RELEASE).jar

export GO_LDFLAGS

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
	grep -v ^LICENSE )
RELEASE_TEMPLATES = $(shell git ls-files | grep "release/templates")
IMAGES = peer orderer ccenv javaenv buildenv testenv zookeeper kafka couchdb tools
RELEASE_PLATFORMS = windows-amd64 darwin-amd64 linux-amd64 linux-ppc64le linux-s390x
RELEASE_PKGS = configtxgen cryptogen configtxlator peer orderer

pkgmap.cryptogen      := $(PKGNAME)/common/tools/cryptogen
pkgmap.configtxgen    := $(PKGNAME)/common/configtx/tool/configtxgen
pkgmap.configtxlator  := $(PKGNAME)/common/tools/configtxlator
pkgmap.peer           := $(PKGNAME)/peer
pkgmap.orderer        := $(PKGNAME)/orderer
pkgmap.block-listener := $(PKGNAME)/examples/events/block-listener
pkgmap.cryptogen      := $(PKGNAME)/common/tools/cryptogen

include docker-env.mk

all: native docker checks

checks: license spelling linter unit-test behave

desk-check: license spelling linter verify behave

.PHONY: spelling
spelling:
	@scripts/check_spelling.sh

.PHONY: license
license:
	@scripts/check_license.sh

.PHONY: gotools
gotools:
	mkdir -p build/bin
	cd gotools && $(MAKE) install BINDIR=$(GOPATH)/bin

.PHONY: gotools-clean
gotools-clean:
	cd gotools && $(MAKE) clean

# This is a legacy target left to satisfy existing CI scripts
membersrvc-image:
	@echo "membersrvc has been removed from this build"

.PHONY: peer
peer: build/bin/peer
peer-docker: build/image/peer/$(DUMMY)

.PHONY: orderer
orderer: build/bin/orderer
orderer-docker: build/image/orderer/$(DUMMY)

.PHONY: configtxgen
configtxgen: GO_TAGS+= nopkcs11
configtxgen: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)
configtxgen: build/bin/configtxgen

configtxlator: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)
configtxlator: build/bin/configtxlator

cryptogen: GO_LDFLAGS=-X $(pkgmap.$(@F))/metadata.Version=$(PROJECT_VERSION)
cryptogen: build/bin/cryptogen

tools-docker: build/image/tools/$(DUMMY)

javaenv: build/image/javaenv/$(DUMMY)

buildenv: build/image/buildenv/$(DUMMY)

build/image/testenv/$(DUMMY): build/image/buildenv/$(DUMMY)
testenv: build/image/testenv/$(DUMMY)

couchdb: build/image/couchdb/$(DUMMY)

kafka: build/image/kafka/$(DUMMY)

zookeeper: build/image/zookeeper/$(DUMMY)

unit-test: unit-test-clean peer-docker testenv couchdb
	cd unit-test && docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

unit-tests: unit-test

verify: unit-test-clean peer-docker testenv couchdb
	cd unit-test && JOB_TYPE=VERIFY docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

# Generates a string to the terminal suitable for manual augmentation / re-issue, useful for running tests by hand
test-cmd:
	@echo "go test -ldflags \"$(GO_LDFLAGS)\""

docker: $(patsubst %,build/image/%/$(DUMMY), $(IMAGES))
native: peer orderer configtxgen cryptogen configtxlator

behave-deps: docker peer build/bin/block-listener configtxgen cryptogen
behave: behave-deps
	@echo "Running behave tests"
	@cd bddtests; behave $(BEHAVE_OPTS)

behave-peer-chaincode: build/bin/peer peer-docker orderer-docker
	@cd peer/chaincode && behave

linter: buildenv
	@echo "LINT: Running code checks.."
	@$(DRUN) $(DOCKER_NS)/fabric-buildenv:$(DOCKER_TAG) ./scripts/golinter.sh

%/chaintool: Makefile
	@echo "Installing chaintool"
	@mkdir -p $(@D)
	curl -fL $(CHAINTOOL_URL) > $@
	chmod +x $@

# We (re)build a package within a docker context but persist the $GOPATH/pkg
# directory so that subsequent builds are faster
build/docker/bin/%: $(PROJECT_FILES)
	$(eval TARGET = ${patsubst build/docker/bin/%,%,${@}})
	@echo "Building $@"
	@mkdir -p build/docker/bin build/docker/$(TARGET)/pkg
	@$(DRUN) \
		-v $(abspath build/docker/bin):/opt/gopath/bin \
		-v $(abspath build/docker/$(TARGET)/pkg):/opt/gopath/pkg \
		$(BASE_DOCKER_NS)/fabric-baseimage:$(BASE_DOCKER_TAG) \
		go install -ldflags "$(DOCKER_GO_LDFLAGS)" $(pkgmap.$(@F))
	@touch $@

build/bin:
	mkdir -p $@

changelog:
	./scripts/changelog.sh v$(PREV_VERSION) v$(BASE_VERSION)

build/docker/gotools/bin/protoc-gen-go: build/docker/gotools

build/docker/gotools: gotools/Makefile
	@mkdir -p $@/bin $@/obj
	@$(DRUN) \
		-v $(abspath $@):/opt/gotools \
		-w /opt/gopath/src/$(PKGNAME)/gotools \
		$(BASE_DOCKER_NS)/fabric-baseimage:$(BASE_DOCKER_TAG) \
		make install BINDIR=/opt/gotools/bin OBJDIR=/opt/gotools/obj

# Both peer and peer-docker depend on ccenv and javaenv (all docker env images it supports).
build/bin/peer: build/image/ccenv/$(DUMMY) build/image/javaenv/$(DUMMY)
build/image/peer/$(DUMMY): build/image/ccenv/$(DUMMY) build/image/javaenv/$(DUMMY)

build/bin/%: $(PROJECT_FILES)
	@mkdir -p $(@D)
	@echo "$@"
	$(CGO_FLAGS) GOBIN=$(abspath $(@D)) go install -tags "$(GO_TAGS)" -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
	@echo "Binary available as $@"
	@touch $@

# payload definitions'
build/image/ccenv/payload:      build/docker/gotools/bin/protoc-gen-go \
				build/bin/chaintool \
				build/goshim.tar.bz2
build/image/javaenv/payload:    build/javashim.tar.bz2 \
				build/protos.tar.bz2 \
				settings.gradle
build/image/peer/payload:       build/docker/bin/peer \
				build/sampleconfig.tar.bz2
build/image/orderer/payload:    build/docker/bin/orderer \
				build/sampleconfig.tar.bz2
build/image/buildenv/payload:   build/gotools.tar.bz2 \
				build/docker/gotools/bin/protoc-gen-go
build/image/testenv/payload:    build/docker/bin/orderer \
				build/docker/bin/peer \
				build/sampleconfig.tar.bz2 \
				images/testenv/install-softhsm2.sh
build/image/zookeeper/payload:  images/zookeeper/docker-entrypoint.sh
build/image/kafka/payload:      images/kafka/docker-entrypoint.sh \
				images/kafka/kafka-run-class.sh
build/image/couchdb/payload:	images/couchdb/docker-entrypoint.sh \
				images/couchdb/local.ini \
				images/couchdb/vm.args
build/image/tools/payload:      build/docker/bin/cryptogen \
	                        build/docker/bin/configtxgen \
	                        build/docker/bin/configtxlator \
				build/docker/bin/peer \
				build/sampleconfig.tar.bz2

build/image/%/payload:
	mkdir -p $@
	cp $^ $@

.PRECIOUS: build/image/%/Dockerfile

build/image/%/Dockerfile: images/%/Dockerfile.in
	@cat $< \
		| sed -e 's/_BASE_NS_/$(BASE_DOCKER_NS)/g' \
		| sed -e 's/_NS_/$(DOCKER_NS)/g' \
		| sed -e 's/_BASE_TAG_/$(BASE_DOCKER_TAG)/g' \
		| sed -e 's/_TAG_/$(DOCKER_TAG)/g' \
		> $@
	@echo LABEL $(BASE_DOCKER_LABEL).version=$(PROJECT_VERSION) \\>>$@
	@echo "     " $(BASE_DOCKER_LABEL).base.version=$(BASEIMAGE_RELEASE)>>$@

build/image/%/$(DUMMY): Makefile build/image/%/payload build/image/%/Dockerfile
	$(eval TARGET = ${patsubst build/image/%/$(DUMMY),%,${@}})
	@echo "Building docker $(TARGET)-image"
	$(DBUILD) -t $(DOCKER_NS)/fabric-$(TARGET) $(@D)
	docker tag $(DOCKER_NS)/fabric-$(TARGET) $(DOCKER_NS)/fabric-$(TARGET):$(DOCKER_TAG)
	@touch $@

build/gotools.tar.bz2: build/docker/gotools
	(cd $</bin && tar -jc *) > $@

build/goshim.tar.bz2: $(GOSHIM_DEPS)
	@echo "Creating $@"
	@tar -jhc -C $(GOPATH)/src $(patsubst $(GOPATH)/src/%,%,$(GOSHIM_DEPS)) > $@

build/sampleconfig.tar.bz2: $(shell find sampleconfig -type f)
	(cd sampleconfig && tar -jc *) > $@

build/javashim.tar.bz2: $(JAVASHIM_DEPS)
build/protos.tar.bz2: $(PROTOS)

build/%.tar.bz2:
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
	@cat $(@D)/../templates/get-byfn.in \
		| sed -e 's/_VERSION_/$(PROJECT_VERSION)/g' \
		> $(@D)/bin/get-byfn.sh
		@chmod +x $(@D)/bin/get-byfn.sh

.PHONY: dist
dist: dist-clean release
	cd release/$(MARCH) && tar -czvf hyperledger-fabric-$(MARCH).$(PROJECT_VERSION).tar.gz *

dist-all: dist-clean release-all $(patsubst %,dist/%, $(RELEASE_PLATFORMS))

dist/windows-amd64:
	cd release/windows-amd64 && tar -czvf hyperledger-fabric-windows-amd64.$(PROJECT_VERSION).tar.gz *

dist/darwin-amd64:
	cd release/darwin-amd64 && tar -czvf hyperledger-fabric-darwin-amd64.$(PROJECT_VERSION).tar.gz *

dist/linux-amd64:
	cd release/linux-amd64 && tar -czvf hyperledger-fabric-linux-amd64.$(PROJECT_VERSION).tar.gz *

dist/linux-ppc64le:
	cd release/linux-ppc64le && tar -czvf hyperledger-fabric-linux-ppc64le.$(PROJECT_VERSION).tar.gz *

dist/linux-s390x:
	cd release/linux-s390x && tar -czvf hyperledger-fabric-linux-s390x.$(PROJECT_VERSION).tar.gz *

.PHONY: protos
protos: buildenv
	@$(DRUN) $(DOCKER_NS)/fabric-buildenv:$(DOCKER_TAG) ./scripts/compile_protos.sh

%-docker-clean:
	$(eval TARGET = ${patsubst %-docker-clean,%,${@}})
	-docker images -q $(DOCKER_NS)/fabric-$(TARGET) | xargs -I '{}' docker rmi -f '{}'
	-@rm -rf build/image/$(TARGET) ||:

docker-clean: $(patsubst %,%-docker-clean, $(IMAGES))

.PHONY: clean
clean: docker-clean unit-test-clean release-clean
	-@rm -rf build ||:

.PHONY: clean-all
clean-all: clean gotools-clean dist-clean release-clean unit-test-clean
	-@rm -rf /var/hyperledger/* ||:

.PHONY: dist-clean
dist-clean:
	-@rm -rf release/windows-amd64/hyperledger-fabric-windows-amd64.$(PROJECT_VERSION).tar.gz ||:
	-@rm -rf release/darwin-amd64/hyperledger-fabric-darwin-amd64.$(PROJECT_VERSION).tar.gz ||:
	-@rm -rf release/linux-amd64/hyperledger-fabric-linux-amd64.$(PROJECT_VERSION).tar.gz ||:
	-@rm -rf release/linux-ppc64le/hyperledger-fabric-linux-ppc64le.$(PROJECT_VERSION).tar.gz ||:
	-@rm -rf release/linux-s390x/hyperledger-fabric-linux-s390x.$(PROJECT_VERSION).tar.gz ||:

%-release-clean:
	$(eval TARGET = ${patsubst %-release-clean,%,${@}})
	-@rm -rf release/$(TARGET)

release-clean: $(patsubst %,%-release-clean, $(RELEASE_PLATFORMS))

.PHONY: unit-test-clean
unit-test-clean:
	cd unit-test && docker-compose down
