# Copyright Greg Haskins All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
FROM _BASE_NS_/fabric-baseos:_BASE_TAG_
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
RUN mkdir -p /var/hyperledger/production $FABRIC_CFG_PATH
COPY payload/peer /usr/local/bin
ADD  payload/sampleconfig.tar.bz2 $FABRIC_CFG_PATH
CMD ["peer","node","start"]
