# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG ALPINE_VER
ARG GO_VER

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
RUN make gotools

FROM golang:${GO_VER}-alpine
RUN apk add --no-cache \
	gcc \
	musl-dev \
	git \
	bash \
	protobuf-dev \
	make;
COPY --from=golang /go/bin /usr/local/bin
ENV GOCACHE "/tmp"
