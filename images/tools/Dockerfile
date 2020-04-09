# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG GO_VER
ARG ALPINE_VER
FROM --platform=$BUILDPLATFORM golang:${GO_VER}-alpine${ALPINE_VER} as golang

RUN apk add --no-cache \
	bash \
	gcc \
	git \
	make \
	musl-dev;

ADD . $GOPATH/src/github.com/hyperledger/fabric
WORKDIR $GOPATH/src/github.com/hyperledger/fabric

FROM golang as tools
ARG BUILDPLATFORM
ARG TARGETPLATFORM
RUN export MAKETARGET=$(echo $TARGETPLATFORM | sed -e 's|/|-|g') && \
	make release/${MAKETARGET}/bin/configtxgen \
		release/${MAKETARGET}/bin/configtxlator \
		release/${MAKETARGET}/bin/cryptogen \
		release/${MAKETARGET}/bin/peer \
		release/${MAKETARGET}/bin/discover \
		release/${MAKETARGET}/bin/idemixgen

FROM golang:${GO_VER}-alpine
# git is required to support `go list -m`
RUN apk add --no-cache \
	bash \
	git \
	jq \
	tzdata;
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
VOLUME /etc/hyperledger/fabric
COPY --from=tools /go/src/github.com/hyperledger/fabric/release/*/bin /usr/local/bin
COPY --from=tools /go/src/github.com/hyperledger/fabric/sampleconfig ${FABRIC_CFG_PATH}
