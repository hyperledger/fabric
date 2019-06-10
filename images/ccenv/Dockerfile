# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
ARG GO_VER
ARG ALPINE_VER
FROM golang:${GO_VER}-alpine${ALPINE_VER} as shim
RUN apk add --no-cache \
	gcc \
	musl-dev \
	git;

ADD . $GOPATH/src/github.com/hyperledger/fabric
WORKDIR $GOPATH/src/github.com/hyperledger/fabric
RUN go get -u github.com/kardianos/govendor \
	&& mkdir $GOPATH/src/input \
	&& cp images/ccenv/main.go $GOPATH/src/input/. \
	&& cd $GOPATH/src/input \
	&& $GOPATH/bin/govendor init \
	&& $GOPATH/bin/govendor add +external github.com/hyperledger/fabric/core/chaincode/shim/... \
	&& rm $GOPATH/src/input/vendor/vendor.json;

FROM golang:${GO_VER}-alpine${ALPINE_VER}
RUN apk add --no-cache \
	g++ \
	gcc \
	musl-dev \
	git;
RUN mkdir -p /chaincode/output \
	&& mkdir -p /chaincode/input;

# add the shim to GOPATH
COPY --from=shim $GOPATH/src/input/vendor $GOPATH/src
RUN addgroup chaincode && adduser -D -h /home/chaincode -G chaincode chaincode
RUN chown -R chaincode:chaincode /chaincode/output \
	&& chown -R chaincode:chaincode /chaincode/input;
USER chaincode
