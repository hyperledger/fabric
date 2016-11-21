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
#   - peer - builds the fabric peer binary
#   - orderer - builds the fabric orderer binary
#   - unit-test - runs the go-test based unit tests
#   - behave - runs the behave test
#   - behave-deps - ensures pre-requisites are availble for running behave manually
#   - gotools - installs go tools like golint
#   - linter - runs all code checks
#   - images[-clean] - ensures all docker images are available[/cleaned]
#   - peer-image[-clean] - ensures the peer-image is available[/cleaned] (for behave, etc)
#   - orderer-image[-clean] - ensures the orderer-image is available[/cleaned] (for behave, etc)
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
CGO_FLAGS = CGO_CFLAGS=" " CGO_LDFLAGS="-lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy"
UID = $(shell id -u)
ARCH=$(shell uname -m)
CHAINTOOL_RELEASE=v0.10.0
BASEIMAGE_RELEASE=$(shell cat ./.baseimage-release)

DOCKER_TAG=$(ARCH)-$(PROJECT_VERSION)
BASE_DOCKER_TAG=$(ARCH)-$(BASEIMAGE_RELEASE)

EXECUTABLES = go docker git curl
K := $(foreach exec,$(EXECUTABLES),\
	$(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH: Check dependencies")))

GOSHIM_DEPS = $(shell ./scripts/goListFiles.sh $(PKGNAME)/core/chaincode/shim | sort | uniq)
JAVASHIM_DEPS =  $(shell git ls-files core/chaincode/shim/java)
PROTOS = $(shell git ls-files *.proto | grep -v vendor)
PROJECT_FILES = $(shell git ls-files)
IMAGES = peer orderer ccenv javaenv

pkgmap.peer           := $(PKGNAME)/peer
pkgmap.orderer        := $(PKGNAME)/orderer
pkgmap.block-listener := $(PKGNAME)/examples/events/block-listener

all: peer orderer checks

checks: linter unit-test behave

.PHONY: gotools
gotools:
	mkdir -p build/bin
	cd gotools && $(MAKE) install BINDIR=$(GOPATH)/bin

gotools-clean:
	cd gotools && $(MAKE) clean

membersrvc-image:
	@echo "membersrvc has been removed from this build"

.PHONY: peer
peer: build/bin/peer
peer-image: build/image/peer/.dummy

.PHONY: orderer
orderer: build/bin/orderer
orderer-image: build/image/orderer/.dummy

unit-test: peer-image gotools
	@./scripts/goUnitTests.sh $(DOCKER_TAG) "$(GO_LDFLAGS)"

unit-tests: unit-test

.PHONY: images
images: $(patsubst %,build/image/%/.dummy, $(IMAGES))

behave-deps: images peer build/bin/block-listener
behave: behave-deps
	@echo "Running behave tests"
	@cd bddtests; behave $(BEHAVE_OPTS)

linter: gotools
	@echo "LINT: Running code checks.."
	@./scripts/golinter.sh

# We (re)build protoc-gen-go from within docker context so that
# we may later inject the binary into a different docker environment
# This is necessary since we cannot guarantee that binaries built
# on the host natively will be compatible with the docker env.
%/bin/protoc-gen-go: Makefile
	@echo "Building $@"
	@mkdir -p $(@D)
	@docker run -i \
		--user=$(UID) \
		-v $(abspath vendor/github.com/golang/protobuf):/opt/gopath/src/github.com/golang/protobuf \
		-v $(abspath $(@D)):/opt/gopath/bin \
		hyperledger/fabric-baseimage:$(BASE_DOCKER_TAG) go install github.com/golang/protobuf/protoc-gen-go

build/bin/chaintool: Makefile
	@echo "Installing chaintool"
	@mkdir -p $(@D)
	curl -L https://github.com/hyperledger/fabric-chaintool/releases/download/$(CHAINTOOL_RELEASE)/chaintool > $@
	chmod +x $@

%/bin/chaintool: build/bin/chaintool
	@mkdir -p $(@D)
	@cp $^ $@

# We (re)build a package within a docker context but persist the $GOPATH/pkg
# directory so that subsequent builds are faster
build/docker/bin/%: $(PROJECT_FILES)
	$(eval TARGET = ${patsubst build/docker/bin/%,%,${@}})
	@echo "Building $@"
	@mkdir -p build/docker/bin build/docker/pkg
	@docker run -i \
		--user=$(UID) \
		-v $(abspath build/docker/bin):/opt/gopath/bin \
		-v $(abspath build/docker/pkg):/opt/gopath/pkg \
		-v $(abspath .):/opt/gopath/src/$(PKGNAME) \
		hyperledger/fabric-baseimage:$(BASE_DOCKER_TAG) \
		go install -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
	@touch $@

build/bin:
	mkdir -p $@

# Both peer and peer-image depend on ccenv-image and javaenv-image (all docker env images it supports)
build/bin/peer: build/image/ccenv/.dummy build/image/javaenv/.dummy
build/image/peer/.dummy: build/image/ccenv/.dummy build/image/javaenv/.dummy

build/bin/%: $(PROJECT_FILES)
	@mkdir -p $(@D)
	@echo "$@"
	$(CGO_FLAGS) GOBIN=$(abspath $(@D)) go install -ldflags "$(GO_LDFLAGS)" $(pkgmap.$(@F))
	@echo "Binary available as $@"
	@touch $@

# payload definitions'
build/image/ccenv/payload:      build/docker/bin/protoc-gen-go \
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
	docker build -t $(PROJECT_NAME)-$(TARGET) $(@D)
	docker tag $(PROJECT_NAME)-$(TARGET) $(PROJECT_NAME)-$(TARGET):$(DOCKER_TAG)
	@touch $@

build/goshim.tar.bz2: $(GOSHIM_DEPS)
	@echo "Creating $@"
	@tar -jhc -C $(GOPATH)/src $(patsubst $(GOPATH)/src/%,%,$(GOSHIM_DEPS)) > $@

build/javashim.tar.bz2: $(JAVASHIM_DEPS)
build/protos.tar.bz2: $(PROTOS)

build/%.tar.bz2:
	@echo "Creating $@"
	@tar -jc $^ > $@

.PHONY: protos
protos: gotools
	./scripts/compile_protos.sh

%-image-clean:
	$(eval TARGET = ${patsubst %-image-clean,%,${@}})
	-docker images -q $(PROJECT_NAME)-$(TARGET) | xargs docker rmi -f
	-@rm -rf build/image/$(TARGET) ||:

images-clean: $(patsubst %,%-image-clean, $(IMAGES))

.PHONY: clean
clean: images-clean
	-@rm -rf build ||:

.PHONY: dist-clean
dist-clean: clean gotools-clean
	-@rm -rf /var/hyperledger/* ||:
