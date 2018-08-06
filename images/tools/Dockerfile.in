# Copyright Greg Haskins All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
FROM _BASE_NS_/fabric-baseimage:_BASE_TAG_ as builder
WORKDIR /opt/gopath
RUN mkdir src && mkdir pkg && mkdir bin
ADD . src/github.com/hyperledger/fabric
WORKDIR /opt/gopath/src/github.com/hyperledger/fabric
ENV EXECUTABLES go git curl
RUN make configtxgen configtxlator cryptogen peer discover idemixgen

FROM _BASE_NS_/fabric-baseimage:_BASE_TAG_
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
RUN apt-get update && apt-get install -y jq
VOLUME /etc/hyperledger/fabric
COPY --from=builder /opt/gopath/src/github.com/hyperledger/fabric/.build/bin /usr/local/bin
COPY --from=builder /opt/gopath/src/github.com/hyperledger/fabric/sampleconfig $FABRIC_CFG_PATH
