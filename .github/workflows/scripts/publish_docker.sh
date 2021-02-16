#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

make docker

docker login --username "${DOCKER_USERNAME}" --password "${DOCKER_PASSWORD}"
for image in baseos peer orderer ccenv tools; do
  for release in ${RELEASE} ${TWO_DIGIT_RELEASE}; do
    docker tag "hyperledger/fabric-${image}" "hyperledger/fabric-${image}:amd64-${release}"
    docker tag "hyperledger/fabric-${image}" "hyperledger/fabric-${image}:${release}"
    docker push "hyperledger/fabric-${image}:amd64-${release}"
    docker push "hyperledger/fabric-${image}:${release}"
  done
done
