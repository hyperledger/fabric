# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG GO_VER
ARG ALPINE_VER
FROM golang:${GO_VER}-alpine${ALPINE_VER}
RUN apk add --no-cache \
	g++ \
	gcc \
	musl-dev \
	git;
RUN mkdir -p /chaincode/output \
	&& mkdir -p /chaincode/input;

RUN addgroup chaincode && adduser -D -h /home/chaincode -G chaincode chaincode
RUN chown -R chaincode:chaincode /chaincode/output /chaincode/input
USER chaincode
