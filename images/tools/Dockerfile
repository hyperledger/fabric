# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
ARG GO_VER
ARG ALPINE_VER
FROM golang:${GO_VER}-alpine as golang

RUN apk add --no-cache \
	gcc \
	musl-dev \
	git \
	bash \
	make;

ADD . $GOPATH/src/github.com/hyperledger/fabric
WORKDIR $GOPATH/src/github.com/hyperledger/fabric
ENV EXECUTABLES go git

FROM golang as tools
RUN make configtxgen configtxlator cryptogen peer discover idemixgen token

FROM golang:${GO_VER}-alpine
RUN apk add --no-cache \
	tzdata \
	jq \
	bash;
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
VOLUME /etc/hyperledger/fabric
COPY --from=tools /go/src/github.com/hyperledger/fabric/.build/bin /usr/local/bin
COPY --from=tools /go/src/github.com/hyperledger/fabric/sampleconfig ${FABRIC_CFG_PATH}
