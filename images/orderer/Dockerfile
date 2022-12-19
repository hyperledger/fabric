#
# Copyright contributors to the Hyperledger Fabric project
#
# SPDX-License-Identifier: Apache-2.0
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at:
#
# 	  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
ARG ALPINE_VER
FROM alpine:${ALPINE_VER}

RUN apk add --no-cache \
	bash \
	curl \
	tzdata

# set up nsswitch.conf for Go's "netgo" implementation
# - https://github.com/golang/go/blob/go1.9.1/src/net/conf.go#L194-L275
# - docker run --rm debian:stretch grep '^hosts:' /etc/nsswitch.conf
RUN echo 'hosts: files dns' > /etc/nsswitch.conf

ARG TARGETOS
ARG TARGETARCH
ARG FABRIC_VER

# copy the release build outputs for the target arch
COPY    release/${TARGETOS}-${TARGETARCH}/bin/orderer /usr/local/bin

ENV     FABRIC_CFG_PATH            /var/hyperledger/fabric/config
ENV     FABRIC_VER                 ${FABRIC_VER}

COPY    sampleconfig/msp           ${FABRIC_CFG_PATH}/msp
COPY    sampleconfig/orderer.yaml  ${FABRIC_CFG_PATH}
COPY    sampleconfig/configtx.yaml ${FABRIC_CFG_PATH}

VOLUME  /etc/hyperledger/fabric
VOLUME  /var/hyperledger
EXPOSE  7050

ENTRYPOINT  [ "orderer" ]
CMD         [ "start" ]