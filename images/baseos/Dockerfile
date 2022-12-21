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

ARG GO_VER
ARG UBUNTU_VER

FROM ubuntu:${UBUNTU_VER} as base

RUN apt update && apt install -y \
    tzdata

RUN     addgroup --gid 500 chaincode
RUN     adduser  --disabled-password --gecos "" --uid 500 --gid 500 --home /home/chaincode chaincode

USER    chaincode