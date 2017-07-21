# Copyright Greg Haskins All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
FROM _BASE_NS_/fabric-baseimage:_BASE_TAG_
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
VOLUME /etc/hyperledger/fabric
ADD  payload/sampleconfig.tar.bz2 $FABRIC_CFG_PATH
COPY payload/cryptogen /usr/local/bin
COPY payload/configtxgen /usr/local/bin
COPY payload/configtxlator /usr/local/bin
COPY payload/peer /usr/local/bin
