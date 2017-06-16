# Copyright Greg Haskins All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
FROM _BASE_NS_/fabric-baseimage:_BASE_TAG_
COPY payload/chaintool payload/protoc-gen-go /usr/local/bin/
ADD payload/goshim.tar.bz2 $GOPATH/src/
RUN mkdir -p /chaincode/input /chaincode/output
