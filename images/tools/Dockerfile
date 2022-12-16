# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG GO_VER
ARG ALPINE_VER
FROM golang:${GO_VER}-alpine${ALPINE_VER} as golang

RUN apk add --no-cache \
	bash \
	binutils-gold \
	gcc \
	git \
	make \
	musl-dev;

ADD . $GOPATH/src/github.com/hyperledger/fabric
WORKDIR $GOPATH/src/github.com/hyperledger/fabric

FROM golang as tools
ARG GO_TAGS
ARG FABRIC_VER
ENV FABRIC_VER ${FABRIC_VER}

# When building in docker, the binary must be linked statically with CGO against libmusl
# in order to avoid SIGSEGV errors when running on alpine.
# This uses the system CC compiler, which has been installed above with apk as musl-dev.
# CC=$(uname -m)-alpine-linux-musl-gcc
#ENV CC         $(uname -m)-alpine-linux-musl-gcc
ENV CGO_ENABLED 1
ENV GO_LDFLAGS  -linkmode external -extldflags '-static'

RUN make tools GO_TAGS=${GO_TAGS}

FROM golang:${GO_VER}-alpine${ALPINE_VER}
# git is required to support `go list -m`
RUN apk add --no-cache \
	bash \
	git \
	jq \
	tzdata;
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
VOLUME /etc/hyperledger/fabric
COPY --from=tools /go/src/github.com/hyperledger/fabric/build/bin /usr/local/bin
COPY --from=tools /go/src/github.com/hyperledger/fabric/sampleconfig ${FABRIC_CFG_PATH}
