#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

make docker

wget -qO "$PWD/manifest-tool" https://github.com/estesp/manifest-tool/releases/download/v1.0.0/manifest-tool-linux-amd64
chmod +x ./manifest-tool

for image in peer orderer ccenv tools; do
  docker login --username "${DOCKER_USERNAME}" --password "${DOCKER_PASSWORD}"
  docker tag "hyperledger/fabric-${image}" "hyperledger/fabric-${image}:amd64-${RELEASE}"
  docker push "hyperledger/fabric-${image}:amd64-${RELEASE}"

  ./manifest-tool push from-args --platforms linux/amd64 --template "hyperledger/fabric-${image}:amd64-${RELEASE}" --target "hyperledger/fabric-${image}:${RELEASE}"
  ./manifest-tool push from-args --platforms linux/amd64 --template "hyperledger/fabric-${image}:amd64-${RELEASE}" --target "hyperledger/fabric-${image}:$(sed 's/..$//' <<< ${RELEASE})"
  ./manifest-tool push from-args --platforms linux/amd64 --template "hyperledger/fabric-${image}:amd64-${RELEASE}" --target "hyperledger/fabric-${image}:latest"
done
