# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG GO_VER
ARG ALPINE_VER

FROM golang:${GO_VER}-alpine${ALPINE_VER}
RUN apk add --no-cache bash git jq tzdata
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
VOLUME /etc/hyperledger/fabric
COPY --chown=0:0 sampleconfig ${FABRIC_CFG_PATH}
COPY --chown=0:0 \
    release/linux-amd64/bin/configtxgen \
    release/linux-amd64/bin/configtxlator \
    release/linux-amd64/bin/cryptogen \
    release/linux-amd64/bin/discover \
    release/linux-amd64/bin/idemixgen \
    release/linux-amd64/bin/peer \
    /usr/local/bin/
