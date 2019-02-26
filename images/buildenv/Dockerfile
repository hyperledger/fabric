# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
ARG GO_VER
ARG ALPINE_VER
FROM golang:${GO_VER}-alpine${ALPINE_VER} as golang
RUN apk add --no-cache \
	gcc \
	musl-dev \
	git \
	bash \
	curl \
	make;
ADD . $GOPATH/src/github.com/hyperledger/fabric
WORKDIR $GOPATH/src/github.com/hyperledger/fabric
ENV EXECUTABLES go git curl
RUN make gotools

FROM golang:${GO_VER}-alpine
RUN apk add --no-cache \
	gcc \
	musl-dev \
	git \
	bash \
	protobuf-dev \
	libtool \
	make;
COPY --from=golang /go/bin /usr/local/bin
ENV GOCACHE "/tmp"
RUN addgroup chaincode && adduser -D -h /home/chaincode -G chaincode chaincode
USER chaincode




