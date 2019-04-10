#!/bin/sh
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#
# This script publishes the fabric docker images to hyperledger dockerhub and nexus3 repositories.
# when publishing the images to dockerhub the values for NS_PULL & NS_PUSH variables set to default values.
# when publishing the images to nexus3 repository the values for NS_PULL & NS_PUSH variables set like below
# NS_PULL=nexus3.hyperledger.org:10001/hyperledger & NS_PUSH=nexus3.hyperledger.org:10002/hyperledger
# since nexus has separate port numbers to pull and push the images to nexus3.

usage() {
  echo "Usage: $0 <username> <password>"
  echo "<username> and <password> credentials for the repository"
  echo "ENV:"
  echo "  NS_PULL=$NS_PULL"
  echo "  NS_PUSH=$NS_PUSH"
  echo "  VERSION=$VERSION"
  echo "  TWO_DIGIT_VERSION=$TWO_DIGIT_VERSION"
  exit 1
}

missing() {
  echo "Error: some image(s) missing from registry"
  echo "ENV:"
  echo "  NS_PULL=$NS_PULL"
  echo "  NS_PUSH=$NS_PUSH"
  echo "  VERSION=$VERSION"
  echo "  TWO_DIGIT_VERSION=$TWO_DIGIT_VERSION"
  exit 1
}

failed() {
  echo "Error: multiarch manifest push failed"
  echo "ENV:"
  echo "  NS_PULL=$NS_PULL"
  echo "  NS_PUSH=$NS_PUSH"
  echo "  VERSION=$VERSION"
  echo "  TWO_DIGIT_VERSION=$TWO_DIGIT_VERSION"
  exit 1
}

USER=${1:-nobody}
PASSWORD=${2:-nohow}
NS_PULL=${NS_PULL:-hyperledger}
NS_PUSH=${NS_PUSH:-hyperledger}
VERSION=${BASE_VERSION:-1.4.1}
TWO_DIGIT_VERSION=${TWO_DIGIT_VERSION:-1.4}

if [ "$#" -ne 2 ]; then
  usage
fi

# verify that manifest-tool is installed and found on PATH
which manifest-tool
if [ "$?" -ne 0 ]; then
  echo "manifest-tool not installed or not found on PATH"
  exit 1
fi

IMAGES="fabric-peer fabric-orderer fabric-ccenv fabric-tools"

# check that all images have been published
for image in ${IMAGES}; do
  docker pull ${NS_PULL}/${image}:amd64-${VERSION} || missing
  docker pull ${NS_PULL}/${image}:s390x-${VERSION} || missing
done

# push the multiarch manifest and tag with $VERSION,$TWO_DIGIT_VERSION and latest tag
for image in ${IMAGES}; do
  manifest-tool --username ${USER} --password ${PASSWORD} push from-args\
   --platforms linux/amd64,linux/s390x --template "${NS_PULL}/${image}:ARCH-${VERSION}"\
   --target "${NS_PUSH}/${image}:${VERSION}"
  manifest-tool --username ${USER} --password ${PASSWORD} push from-args\
   --platforms linux/amd64,linux/s390x --template "${NS_PULL}/${image}:ARCH-${VERSION}"\
   --target "${NS_PUSH}/${image}:latest"
  manifest-tool --username ${USER} --password ${PASSWORD} push from-args\
   --platforms linux/amd64,linux/s390x --template "${NS_PULL}/${image}:ARCH-${VERSION}"\
   --target "${NS_PUSH}/${image}:${TWO_DIGIT_VERSION}"
done

# test that manifest is working as expected
for image in ${IMAGES}; do
  docker pull ${NS_PULL}/${image}:${VERSION} || failed
  docker pull ${NS_PULL}/${image}:${TWO_DIGIT_VERSION} || failed
  docker pull ${NS_PULL}/${image}:latest || failed
done

echo "Successfully pushed multiarch manifest"
exit 0
