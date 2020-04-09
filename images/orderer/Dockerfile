# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG GO_VER
ARG ALPINE_VER
FROM alpine:${ALPINE_VER} as base
RUN apk add --no-cache tzdata

FROM --platform=$BUILDPLATFORM golang:${GO_VER}-alpine${ALPINE_VER} as golang
RUN apk add --no-cache \
	gcc \
	musl-dev \
	git \
	bash \
	make;
ADD . $GOPATH/src/github.com/hyperledger/fabric
WORKDIR $GOPATH/src/github.com/hyperledger/fabric

FROM golang as orderer
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG GO_TAGS
RUN export MAKETARGET=$(echo $TARGETPLATFORM | sed -e 's|/|-|g') && \
	make release/${MAKETARGET}/bin/orderer GO_TAGS=${GO_TAGS}

FROM base
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
VOLUME /etc/hyperledger/fabric
VOLUME /var/hyperledger
COPY --from=orderer /go/src/github.com/hyperledger/fabric/release/*/bin /usr/local/bin
COPY --from=orderer /go/src/github.com/hyperledger/fabric/sampleconfig/msp ${FABRIC_CFG_PATH}/msp
COPY --from=orderer /go/src/github.com/hyperledger/fabric/sampleconfig/orderer.yaml ${FABRIC_CFG_PATH}
COPY --from=orderer /go/src/github.com/hyperledger/fabric/sampleconfig/configtx.yaml ${FABRIC_CFG_PATH}
EXPOSE 7050
CMD ["orderer"]
