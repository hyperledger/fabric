# Licensed to the Apache Software Foundation (ASF) under one
# or more contributor license agreements.  See the NOTICE file
# distributed with this work for additional information
# regarding copyright ownership.  The ASF licenses this file
# to you under the Apache License, Version 2.0 (the
# "License"); you may not use this file except in compliance
# with the License.  You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.
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
GO_LDFLAGS = -X $(PKGNAME)/metadata.Version=$(PROJECT_VERSION)
CGO_FLAGS = CGO_CFLAGS=" "
ARCH=$(shell uname -m)
OS=$(shell uname)
CHAINTOOL_RELEASE=v0.10.0
BASEIMAGE_RELEASE=$(shell cat ./.baseimage-release)

export GO_LDFLAGS

DOCKER_TAG=$(ARCH)-$(PROJECT_VERSION)
BASE_DOCKER_TAG=$(ARCH)-$(BASEIMAGE_RELEASE)

ifneq ($(OS),Darwin)
DOCKER_FLAGS=--user=$(shell id -u)
endif

ifneq ($(http_proxy),)
DOCKER_ARGS_PROXY+=--build-arg http_proxy=$(http_proxy)
DOCKER_FLAGS+=-e http_proxy=$(http_proxy)
endif
ifneq ($(https_proxy),)
DOCKER_ARGS_PROXY+=--build-arg https_proxy=$(https_proxy)
DOCKER_FLAGS+=-e https_proxy=$(https_proxy)
endif
ifneq ($(HTTP_PROXY),)
DOCKER_ARGS_PROXY+=--build-arg HTTP_PROXY=$(HTTP_PROXY)
DOCKER_FLAGS+=-e HTTP_PROXY=$(HTTP_PROXY)
endif
ifneq ($(HTTPS_PROXY),)
DOCKER_ARGS_PROXY+=--build-arg HTTPS_PROXY=$(HTTPS_PROXY)
DOCKER_FLAGS+=-e HTTPS_PROXY=$(HTTPS_PROXY)
endif
ifneq ($(no_proxy),)
DOCKER_ARGS_PROXY+=--build-arg no_proxy=$(no_proxy)
DOCKER_FLAGS+=-e no_proxy=$(no_proxy)
endif
ifneq ($(NO_PROXY),)
DOCKER_ARGS_PROXY+=--build-arg NO_PROXY=$(NO_PROXY)
DOCKER_FLAGS+=-e NO_PROXY=$(NO_PROXY)
endif

DRUN = docker run -i --rm $(DOCKER_FLAGS) \
	-v $(abspath .):/opt/gopath/src/$(PKGNAME) \
	-w /opt/gopath/src/$(PKGNAME)

EXECUTABLES = go docker git curl
K := $(foreach exec,$(EXECUTABLES),\
	$(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH: Check dependencies")))

GOSHIM_DEPS = $(shell ./scripts/goListFiles.sh $(PKGNAME)/core/chaincode/shim | sort | uniq)
JAVASHIM_DEPS =  $(shell git ls-files core/chaincode/shim/java)
PROTOS = $(shell git ls-files *.proto | grep -v vendor)
PROJECT_FILES = $(shell git ls-files)
IMAGES = peer orderer ccenv javaenv testenv

pkgmap.peer           := $(PKGNAME)/peer
pkgmap.orderer        := $(PKGNAME)/orderer
pkgmap.block-listener := $(PKGNAME)/examples/events/block-listener

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
peer-docker: build/image/peer/.dummy

.PHONY: orderer
orderer: build/bin/orderer
orderer-docker: build/image/orderer/.dummy

testenv: build/image/testenv/.dummy

unit-test: peer-docker testenv
	cd unit-test && docker-compose up --abort-on-container-exit --force-recreate && docker-compose down

unit-tests: unit-test

docker: $(patsubst %,build/image/%/.dummy, $(IMAGES))
native: peer orderer

behave-deps: docker peer build/bin/block-listener
behave: behave-deps
	@echo "Running behave tests"
	@cd bddtests; behave $(BEHAVE_OPTS)

linter: testenv
	@echo "LINT: Running code checks.."
	@$(DRUN) hyperledger/fabric-testenv:$(DOCKER_TAG) ./scripts/golinter.sh

%/chaintool: Makefile
	@echo "Installing chaintool"
	@mkdir -p $(@D)
	curl -L https://github.com/hyperledger/fabric-chaintool/releases/download/$(CHAINTOOL_RELEASE)/chaintool > $@
	chmod +x $@

# We (re)build a package within a docker context but persist the $GOPATH/pkg
# directory so that subsequent builds are faster
build/docker/bin/%: $(PROJECT_FILES)
	$(eval TARGET = ${patsubst build/docker/bin/%,%,${@}})
	@echo "Building $@"
	@mkdir -p build/docker/bin build/docker/pkg
	@$(DRUN) \
		-v $(abspath build/docker/bin):/opt/gopath/bin \
		-v $(abspath build/docker/pkg):/opt/gopath/pkg \
		hyperledger/fabric-baseimage:$(BASE_DOCKER_TAG) \
		go install -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
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

# Both peer and peer-docker depend on ccenv and javaenv (all docker env images it supports)
build/bin/peer: build/image/ccenv/.dummy build/image/javaenv/.dummy
build/image/peer/.dummy: build/image/ccenv/.dummy build/image/javaenv/.dummy

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
				msp/peer-config.json
build/image/orderer/payload:    build/docker/bin/orderer \
				orderer/orderer.yaml
build/image/testenv/payload:    build/gotools.tar.bz2

build/image/%/payload:
	mkdir -p $@
	cp $^ $@

build/image/%/.dummy: Makefile build/image/%/payload
	$(eval TARGET = ${patsubst build/image/%/.dummy,%,${@}})
	@echo "Building docker $(TARGET)-image"
	@cat images/$(TARGET)/Dockerfile.in \
		| sed -e 's/_BASE_TAG_/$(BASE_DOCKER_TAG)/g' \
		| sed -e 's/_TAG_/$(DOCKER_TAG)/g' \
		> $(@D)/Dockerfile
	docker build $(DOCKER_ARGS_PROXY) -t $(PROJECT_NAME)-$(TARGET) $(@D)
	docker tag $(PROJECT_NAME)-$(TARGET) $(PROJECT_NAME)-$(TARGET):$(DOCKER_TAG)
	@touch $@

build/gotools.tar.bz2: build/docker/gotools
	(cd $</bin && tar -jc *) > $@

build/goshim.tar.bz2: $(GOSHIM_DEPS)
	@echo "Creating $@"
	@tar -jhc -C $(GOPATH)/src $(patsubst $(GOPATH)/src/%,%,$(GOSHIM_DEPS)) > $@

build/javashim.tar.bz2: $(JAVASHIM_DEPS)
build/protos.tar.bz2: $(PROTOS)

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
