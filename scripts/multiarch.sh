#!/bin/sh
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

usage() {
  echo "Usage: $0 <username> <password>"
  echo "<username> and <password> credentials for the repository"
  echo "ENV:"
  echo "  NS=$NS"
  echo "  VERSION=$VERSION"
  exit 1
}

missing() {
  echo "Error: some image(s) missing from registry"
  echo "ENV:"
  echo "  NS=$NS"
  echo "  VERSION=$VERSION"
  exit 1
}

failed() {
  echo "Error: multiarch manifest push failed"
  echo "ENV:"
  echo "  NS=$NS"
  echo "  VERSION=$VERSION"
  exit 1
}

USER=${1:-nobody}
PASSWORD=${2:-nohow}
NS=${NS:-hyperledger}
VERSION=${BASE_VERSION:-1.1.0}

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
  docker pull ${NS}/${image}:amd64-${VERSION} || missing
  docker pull ${NS}/${image}:s390x-${VERSION} || missing
done

# push the multiarch manifest and tag with just $VERSION and 'latest'
for image in ${IMAGES}; do
  manifest-tool --username ${USER} --password ${PASSWORD} push from-args\
   --platforms linux/amd64,linux/s390x --template "${NS}/${image}:ARCH-${VERSION}"\
   --target "${NS}/${image}:${VERSION}"
  manifest-tool --username ${USER} --password ${PASSWORD} push from-args\
   --platforms linux/amd64,linux/s390x --template "${NS}/${image}:ARCH-${VERSION}"\
   --target "${NS}/${image}:latest"
done

# test that manifest is working as expected
for image in ${IMAGES}; do
  docker pull ${NS}/${image}:${VERSION} || failed
  docker pull ${NS}/${image}:latest || failed
done

echo "Successfully pushed multiarch manifest"
exit 0
