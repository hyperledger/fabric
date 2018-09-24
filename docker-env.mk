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

ifneq ($(shell uname),Darwin)
DOCKER_RUN_FLAGS=--user=$(shell id -u)
endif

ifeq ($(shell uname -m),s390x)
ifneq ($(shell id -u),0)
DOCKER_RUN_FLAGS+=-v /etc/passwd:/etc/passwd:ro
endif
endif

ifneq ($(http_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg 'http_proxy=$(http_proxy)'
DOCKER_RUN_FLAGS+=-e 'http_proxy=$(http_proxy)'
endif
ifneq ($(https_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg 'https_proxy=$(https_proxy)'
DOCKER_RUN_FLAGS+=-e 'https_proxy=$(https_proxy)'
endif
ifneq ($(HTTP_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg 'HTTP_PROXY=$(HTTP_PROXY)'
DOCKER_RUN_FLAGS+=-e 'HTTP_PROXY=$(HTTP_PROXY)'
endif
ifneq ($(HTTPS_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg 'HTTPS_PROXY=$(HTTPS_PROXY)'
DOCKER_RUN_FLAGS+=-e 'HTTPS_PROXY=$(HTTPS_PROXY)'
endif
ifneq ($(no_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg 'no_proxy=$(no_proxy)'
DOCKER_RUN_FLAGS+=-e 'no_proxy=$(no_proxy)'
endif
ifneq ($(NO_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg 'NO_PROXY=$(NO_PROXY)'
DOCKER_RUN_FLAGS+=-e 'NO_PROXY=$(NO_PROXY)'
endif

DRUN = docker run -i --rm $(DOCKER_RUN_FLAGS) \
	-v $(abspath .):/opt/gopath/src/$(PKGNAME) \
	-w /opt/gopath/src/$(PKGNAME)

DBUILD = docker build $(DOCKER_BUILD_FLAGS)

BASE_DOCKER_NS ?= hyperledger
BASE_DOCKER_TAG=$(ARCH)-$(BASEIMAGE_RELEASE)

DOCKER_NS ?= hyperledger
DOCKER_TAG=$(ARCH)-$(PROJECT_VERSION)
PREV_TAG=$(ARCH)-$(PREV_VERSION)

BASE_DOCKER_LABEL=org.hyperledger.fabric

DOCKER_DYNAMIC_LINK ?= false
DOCKER_GO_LDFLAGS += $(GO_LDFLAGS)

ifeq ($(DOCKER_DYNAMIC_LINK),false)
DOCKER_GO_LDFLAGS += -linkmode external -extldflags '-static -lpthread'
endif

#
# What is a .dummy file?
#
# Make is designed to work with files.  It uses the presence (or lack thereof)
# and timestamps of files when deciding if a given target needs to be rebuilt.
# Docker containers throw a wrench into the works because the output of docker
# builds do not translate into standard files that makefile rules can evaluate.
# Therefore, we have to fake it.  We do this by constructioning our rules such
# as
#       my-docker-target/.dummy:
#              docker build ...
#              touch $@
#
# If the docker-build succeeds, the touch operation creates/updates the .dummy
# file.  If it fails, the touch command never runs.  This means the .dummy
# file follows relatively 1:1 with the underlying container.
#
# This isn't perfect, however.  For instance, someone could delete a docker
# container using docker-rmi outside of the build, and make would be fooled
# into thinking the dependency is statisfied when it really isn't.  This is
# our closest approximation we can come up with.
#
# As an aside, also note that we incorporate the version number in the .dummy
# file to differentiate different tags to fix FAB-1145
#
DUMMY = .dummy-$(DOCKER_TAG)
