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

ifneq ($(shell uname),Darwin)
DOCKER_RUN_FLAGS=--user=$(shell id -u)
endif

ifneq ($(http_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg http_proxy=$(http_proxy)
DOCKER_RUN_FLAGS+=-e http_proxy=$(http_proxy)
endif
ifneq ($(https_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg https_proxy=$(https_proxy)
DOCKER_RUN_FLAGS+=-e https_proxy=$(https_proxy)
endif
ifneq ($(HTTP_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg HTTP_PROXY=$(HTTP_PROXY)
DOCKER_RUN_FLAGS+=-e HTTP_PROXY=$(HTTP_PROXY)
endif
ifneq ($(HTTPS_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg HTTPS_PROXY=$(HTTPS_PROXY)
DOCKER_RUN_FLAGS+=-e HTTPS_PROXY=$(HTTPS_PROXY)
endif
ifneq ($(no_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg no_proxy=$(no_proxy)
DOCKER_RUN_FLAGS+=-e no_proxy=$(no_proxy)
endif
ifneq ($(NO_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg NO_PROXY=$(NO_PROXY)
DOCKER_RUN_FLAGS+=-e NO_PROXY=$(NO_PROXY)
endif

DRUN = docker run -i --rm $(DOCKER_RUN_FLAGS) \
	-v $(abspath .):/opt/gopath/src/$(PKGNAME) \
	-w /opt/gopath/src/$(PKGNAME)

DBUILD = docker build $(DOCKER_BUILD_FLAGS)

DOCKER_TAG=$(ARCH)-$(PROJECT_VERSION)
BASE_DOCKER_TAG=$(ARCH)-$(BASEIMAGE_RELEASE)

DOCKER_GO_LDFLAGS += $(GO_LDFLAGS)
DOCKER_GO_LDFLAGS += -linkmode external -extldflags '-static -lpthread'



