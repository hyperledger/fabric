# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

ARG ALPINE_VER

FROM alpine:${ALPINE_VER}
RUN apk add --no-cache tzdata
ENV FABRIC_CFG_PATH /etc/hyperledger/fabric
VOLUME /etc/hyperledger/fabric
VOLUME /var/hyperledger
COPY --chown=0:0 release/linux-amd64/bin/peer /usr/local/bin
COPY --chown=0:0 sampleconfig/msp ${FABRIC_CFG_PATH}/msp
COPY --chown=0:0 sampleconfig/core.yaml ${FABRIC_CFG_PATH}
EXPOSE 7051
CMD ["peer","node","start"]
