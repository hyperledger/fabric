# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG GO_VER
ARG ALPINE_VER
FROM golang:${GO_VER}-alpine${ALPINE_VER}
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories   && apk add --no-cache \
	g++ \
	gcc \
	git \
	musl-dev

RUN mkdir -p /chaincode/output /chaincode/input
RUN addgroup -g 500 chaincode && adduser -u 500 -D -h /home/chaincode -G chaincode chaincode
RUN chown -R chaincode:chaincode /chaincode
ENV GOPROXY=https://goproxy.cn
USER chaincode
