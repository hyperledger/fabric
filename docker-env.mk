# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ifneq ($(http_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg 'http_proxy=$(http_proxy)'
endif
ifneq ($(https_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg 'https_proxy=$(https_proxy)'
endif
ifneq ($(HTTP_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg 'HTTP_PROXY=$(HTTP_PROXY)'
endif
ifneq ($(HTTPS_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg 'HTTPS_PROXY=$(HTTPS_PROXY)'
endif
ifneq ($(no_proxy),)
DOCKER_BUILD_FLAGS+=--build-arg 'no_proxy=$(no_proxy)'
endif
ifneq ($(NO_PROXY),)
DOCKER_BUILD_FLAGS+=--build-arg 'NO_PROXY=$(NO_PROXY)'
endif

DOCKER_BUILD ?= docker build --force-rm
DBUILD = $(DOCKER_BUILD) $(DOCKER_BUILD_FLAGS)

DOCKER_NS ?= hyperledger
DOCKER_TAG=$(ARCH)-$(PROJECT_VERSION)

BASE_DOCKER_LABEL=org.hyperledger.fabric

#
# What is a .dummy file?
#
# Make is designed to work with files.  It uses the presence (or lack thereof)
# and timestamps of files when deciding if a given target needs to be rebuilt.
# Docker containers throw a wrench into the works because the output of docker
# builds do not translate into standard files that makefile rules can evaluate.
# Therefore, we have to fake it.  We do this by constructing our rules such
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
# into thinking the dependency is satisfied when it really isn't.  This is
# our closest approximation we can come up with.
#
# As an aside, also note that we incorporate the version number in the .dummy
# file to differentiate different tags to fix FAB-1145
#
DUMMY = .dummy-$(DOCKER_TAG)
