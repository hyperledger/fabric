# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# -------------------------------------------------------------
# This makefile defines the following targets
#
#   - all (default) - builds all targets and runs all tests/checks
#   - checks - runs all tests/checks
#   - peer - builds a native fabric peer binary
#   - orderer - builds a native fabric orderer binary
#   - unit-test - runs the go-test based unit tests
#   - behave - runs the behave test
#   - behave-deps - ensures pre-requisites are availble for running behave manually
#   - gotools - installs go tools like golint
#   - linter - runs all code checks
#   - native - ensures all native binaries are available
#   - docker[-clean] - ensures all docker images are available[/cleaned]
#   - peer-docker[-clean] - ensures the peer container is available[/cleaned]
#   - orderer-docker[-clean] - ensures the orderer container is available[/cleaned]
#   - protos - generate all protobuf artifacts based on .proto files
#   - clean - cleans the build area
#   - dist-clean - superset of 'clean' that also removes persistent state

PROJECT_NAME   = hyperledger/fabric
BASE_VERSION   = 0.7.0
IS_RELEASE     = false

ifneq ($(IS_RELEASE),true)
EXTRA_VERSION ?= snapshot-$(shell git rev-parse --short HEAD)
PROJECT_VERSION=$(BASE_VERSION)-$(EXTRA_VERSION)
else
PROJECT_VERSION=$(BASE_VERSION)
endif

PKGNAME = github.com/$(PROJECT_NAME)
GO_LDFLAGS = -X $(PKGNAME)/common/metadata.Version=$(PROJECT_VERSION)
CGO_FLAGS = CGO_CFLAGS=" "
ARCH=$(shell uname -m)
CHAINTOOL_RELEASE=v0.10.1
BASEIMAGE_RELEASE=$(shell cat ./.baseimage-release)

CHAINTOOL_URL ?= https://github.com/hyperledger/fabric-chaintool/releases/download/$(CHAINTOOL_RELEASE)/chaintool

export GO_LDFLAGS

EXECUTABLES = go docker git curl
K := $(foreach exec,$(EXECUTABLES),\
	$(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH: Check dependencies")))

GOSHIM_DEPS = $(shell ./scripts/goListFiles.sh $(PKGNAME)/core/chaincode/shim | sort | uniq)
JAVASHIM_DEPS =  $(shell git ls-files core/chaincode/shim/java)
PROTOS = $(shell git ls-files *.proto | grep -v vendor)
MSP_SAMPLECONFIG = $(shell git ls-files msp/sampleconfig/*.pem)
GENESIS_SAMPLECONFIG = $(shell git ls-files common/configtx/test/*.template)
PROJECT_FILES = $(shell git ls-files)
IMAGES = peer orderer ccenv javaenv testenv zookeeper kafka

pkgmap.peer           := $(PKGNAME)/peer
pkgmap.orderer        := $(PKGNAME)/orderer
pkgmap.block-listener := $(PKGNAME)/examples/events/block-listener

include docker-env.mk

all: native docker checks

checks: linter unit-test behave

.PHONY: gotools
gotools:
	mkdir -p build/bin
	cd gotools && $(MAKE) install BINDIR=$(GOPATH)/bin

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

testenv: build/image/testenv/$(DUMMY)

unit-test: peer-docker testenv
	cd unit-test && docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

unit-tests: unit-test

docker: $(patsubst %,build/image/%/$(DUMMY), $(IMAGES))
native: peer orderer

BEHAVE_ENVIRONMENTS = kafka orderer orderer-1-kafka-1 orderer-1-kafka-3
BEHAVE_ENVIRONMENT_TARGETS = $(patsubst %,bddtests/environments/%, $(BEHAVE_ENVIRONMENTS))
.PHONY: behave-environments $(BEHAVE_ENVIRONMENT_TARGETS)
behave-environments: $(BEHAVE_ENVIRONMENT_TARGETS)
$(BEHAVE_ENVIRONMENT_TARGETS):
	@docker-compose --file $@/docker-compose.yml build

behave-deps: docker peer build/bin/block-listener behave-environments
behave: behave-deps
	@echo "Running behave tests"
	@cd bddtests; behave $(BEHAVE_OPTS)

linter: testenv
	@echo "LINT: Running code checks.."
	@$(DRUN) hyperledger/fabric-testenv:$(DOCKER_TAG) ./scripts/golinter.sh

%/chaintool: Makefile
	@echo "Installing chaintool"
	@mkdir -p $(@D)
	curl -L $(CHAINTOOL_URL) > $@
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
		hyperledger/fabric-baseimage:$(BASE_DOCKER_TAG) \
		go install -ldflags "$(DOCKER_GO_LDFLAGS)" $(pkgmap.$(@F))
	@touch $@

build/bin:
	mkdir -p $@

build/docker/gotools/bin/protoc-gen-go: build/docker/gotools

build/docker/gotools: gotools/Makefile
	@mkdir -p $@/bin $@/obj
	@$(DRUN) \
		-v $(abspath $@):/opt/gotools \
		-w /opt/gopath/src/$(PKGNAME)/gotools \
		hyperledger/fabric-baseimage:$(BASE_DOCKER_TAG) \
		make install BINDIR=/opt/gotools/bin OBJDIR=/opt/gotools/obj

# Both peer and peer-docker depend on ccenv and javaenv (all docker env images it supports).
build/bin/peer: build/image/ccenv/$(DUMMY) build/image/javaenv/$(DUMMY)
build/image/peer/$(DUMMY): build/image/ccenv/$(DUMMY) build/image/javaenv/$(DUMMY)

build/bin/%: $(PROJECT_FILES)
	@mkdir -p $(@D)
	@echo "$@"
	$(CGO_FLAGS) GOBIN=$(abspath $(@D)) go install -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
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
				peer/core.yaml \
				build/msp-sampleconfig.tar.bz2 \
				build/genesis-sampleconfig.tar.bz2
build/image/orderer/payload:    build/docker/bin/orderer \
				build/msp-sampleconfig.tar.bz2 \
				orderer/orderer.yaml
build/image/testenv/payload:    build/gotools.tar.bz2 \
				build/docker/bin/orderer \
				orderer/orderer.yaml \
				build/docker/bin/peer \
				peer/core.yaml \
				build/msp-sampleconfig.tar.bz2 \
				images/testenv/install-softhsm2.sh
build/image/zookeeper/payload:  images/zookeeper/docker-entrypoint.sh
build/image/kafka/payload:      images/kafka/docker-entrypoint.sh \
				images/kafka/kafka-run-class.sh

build/image/%/payload:
	mkdir -p $@
	cp $^ $@

.PRECIOUS: build/image/%/Dockerfile

build/image/%/Dockerfile: images/%/Dockerfile.in
	@cat $< \
		| sed -e 's/_BASE_TAG_/$(BASE_DOCKER_TAG)/g' \
		| sed -e 's/_TAG_/$(DOCKER_TAG)/g' \
		> $@

build/image/%/$(DUMMY): Makefile build/image/%/payload build/image/%/Dockerfile
	$(eval TARGET = ${patsubst build/image/%/$(DUMMY),%,${@}})
	@echo "Building docker $(TARGET)-image"
	$(DBUILD) -t $(PROJECT_NAME)-$(TARGET) $(@D)
	docker tag $(PROJECT_NAME)-$(TARGET) $(PROJECT_NAME)-$(TARGET):$(DOCKER_TAG)
	@touch $@

build/gotools.tar.bz2: build/docker/gotools
	(cd $</bin && tar -jc *) > $@

build/goshim.tar.bz2: $(GOSHIM_DEPS)
	@echo "Creating $@"
	@tar -jhc -C $(GOPATH)/src $(patsubst $(GOPATH)/src/%,%,$(GOSHIM_DEPS)) > $@

build/javashim.tar.bz2: $(JAVASHIM_DEPS)
build/protos.tar.bz2: $(PROTOS)
build/msp-sampleconfig.tar.bz2: $(MSP_SAMPLECONFIG)
build/genesis-sampleconfig.tar.bz2: $(GENESIS_SAMPLECONFIG)

build/%.tar.bz2:
	@echo "Creating $@"
	@tar -jc $^ > $@

.PHONY: protos
protos: testenv
	@$(DRUN) hyperledger/fabric-testenv:$(DOCKER_TAG) ./scripts/compile_protos.sh

%-docker-clean:
	$(eval TARGET = ${patsubst %-docker-clean,%,${@}})
	-docker images -q $(PROJECT_NAME)-$(TARGET) | xargs -I '{}' docker rmi -f '{}'
	-@rm -rf build/image/$(TARGET) ||:

docker-clean: $(patsubst %,%-docker-clean, $(IMAGES))

.PHONY: clean
clean: docker-clean
	-@rm -rf build ||:

.PHONY: dist-clean
dist-clean: clean gotools-clean
	-@rm -rf /var/hyperledger/* ||:
