#!/bin/sh
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -e

usage() {
    echo "Usage: $0 [-g] | [-p <username> <password>"]
    echo
    echo "where:"
    echo "-g generate manifests only"
    echo "-p generate and push manifests requires username and password"
    echo "<username> and <password> credentials for the repository"
    exit 1
}

PUSH=1
if [ "$#" -ne 3 ]; then
    if [ "$1" != "-g" ]; then
        usage
    fi
    PUSH=0
fi

USER=${2:-nobody}
PASSWORD=${3:-nohow}

NS=hyperledger
VERSION=1.1.0
BASE_VERSION=0.4.7
mkdir -p manifests

for image in fabric-peer fabric-orderer fabric-ccenv fabric-tools fabric-ca; do
    DOC=manifests/manifest-${image}-${VERSION}.yaml
    echo "generating: ${DOC}"
    echo "image: ${NS}/${image}:latest" > ${DOC}
    echo "tags: ['${VERSION}', 'latest']" >> $DOC
    echo "manifests:" >> ${DOC}
    echo "  -" >> ${DOC}
    echo "    image: ${NS}/${image}:ppc64le-${VERSION}" >> ${DOC}
    echo "    platform:" >> ${DOC}
    echo "      architecture: ppc64le" >> ${DOC}
    echo "      os: linux" >> ${DOC}
    echo "  -" >> ${DOC}
    echo "    image: ${NS}/${image}:x86_64-${VERSION}" >> ${DOC}
    echo "    platform:" >> ${DOC}
    echo "      architecture: amd64" >> ${DOC}
    echo "      os: linux" >> ${DOC}
    echo "  -" >> ${DOC}
    echo "    image: ${NS}/${image}:s390x-${VERSION}" >> ${DOC}
    echo "    platform:" >> ${DOC}
    echo "      architecture: s390x" >> ${DOC}
    echo "      os: linux" >> ${DOC}
    if [ "$PUSH" -eq 1 ];then
        echo "pushing manifest-list from spec: ${DOC}"
        manifest-tool --username ${USER} --password ${PASSWORD} push from-spec ${DOC}
    fi
done

for image in fabric-baseos fabric-baseimage fabric-kafka fabric-zookeeper fabric-couchdb; do
    DOC=manifests/manifest-${image}-${BASE_VERSION}.yaml
    echo "generating: ${DOC}"
    echo "image: ${NS}/${image}:latest" > ${DOC}
    echo "tags: ['${BASE_VERSION}', 'latest']" >> $DOC
    echo "manifests:" >> ${DOC}
    echo "  -" >> ${DOC}
    echo "    image: ${NS}/${image}:ppc64le-${BASE_VERSION}" >> ${DOC}
    echo "    platform:" >> ${DOC}
    echo "      architecture: ppc64le" >> ${DOC}
    echo "      os: linux" >> ${DOC}
    echo "  -" >> ${DOC}
    echo "    image: ${NS}/${image}:x86_64-${BASE_VERSION}" >> ${DOC}
    echo "    platform:" >> ${DOC}
    echo "      architecture: amd64" >> ${DOC}
    echo "      os: linux" >> ${DOC}
    echo "  -" >> ${DOC}
    echo "    image: ${NS}/${image}:s390x-${BASE_VERSION}" >> ${DOC}
    echo "    platform:" >> ${DOC}
    echo "      architecture: s390x" >> ${DOC}
    echo "      os: linux" >> ${DOC}
    if [ "$PUSH" -eq 1 ];then
        echo "pushing manifest-list from spec: ${DOC}"
        manifest-tool --username ${USER} --password ${PASSWORD} push from-spec ${DOC}
    fi
done

exit 0
