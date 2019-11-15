#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# if version not passed in, default to latest released version
VERSION=1.4.3
# if ca version not passed in, default to latest released version
CA_VERSION=1.4.3
# current version of thirdparty images (couchdb, kafka and zookeeper) released
THIRDPARTY_IMAGE_VERSION=0.4.15
ARCH=$(echo "$(uname -s|tr '[:upper:]' '[:lower:]'|sed 's/mingw64_nt.*/windows/')-$(uname -m | sed 's/x86_64/amd64/g')")
MARCH=$(uname -m)

printHelp() {
    echo "Usage: bootstrap.sh [version [ca_version [thirdparty_version]]] [options]"
    echo
    echo "options:"
    echo "-h : this help"
    echo "-d : bypass docker image download"
    echo "-s : bypass fabric-samples repo clone"
    echo "-b : bypass download of platform-specific binaries"
    echo
    echo "e.g. bootstrap.sh 1.4.3 -s"
    echo "would download docker images and binaries for version 1.4.3"
}

# dockerFabricPull() pulls docker images from fabric and chaincode repositories
# note, if a docker image doesn't exist for a requested release, it will simply
# be skipped, since this script doesn't terminate upon errors.
dockerFabricPull() {
    local FABRIC_TAG=$1
    for IMAGES in peer orderer ccenv tools baseos nodeenv javaenv; do
        echo "==> FABRIC IMAGE: $IMAGES"
        echo
        docker pull "hyperledger/fabric-$IMAGES:$FABRIC_TAG"
        docker tag "hyperledger/fabric-$IMAGES:$FABRIC_TAG" "hyperledger/fabric-$IMAGES"
    done
}

dockerThirdPartyImagesPull() {
    local THIRDPARTY_TAG=$1
    for IMAGES in couchdb kafka zookeeper; do
        echo "==> THIRDPARTY DOCKER IMAGE: $IMAGES"
        echo
        docker pull "hyperledger/fabric-$IMAGES:$THIRDPARTY_TAG"
        docker tag "hyperledger/fabric-$IMAGES:$THIRDPARTY_TAG" "hyperledger/fabric-$IMAGES"
    done
}

dockerCaPull() {
    local CA_TAG=$1
    echo "==> FABRIC CA IMAGE"
    echo
    docker pull "hyperledger/fabric-ca:$CA_TAG"
    docker tag "hyperledger/fabric-ca:$CA_TAG" "hyperledger/fabric-ca"
}

samplesInstall() {
    # clone (if needed) hyperledger/fabric-samples and checkout corresponding
    # version to the binaries and docker images to be downloaded
    if [ -d first-network ]; then
        # if we are in the fabric-samples repo, checkout corresponding version
        echo "===> Checking out v${VERSION} of hyperledger/fabric-samples"
        git checkout v${VERSION}
    elif [ -d fabric-samples ]; then
        # if fabric-samples repo already cloned and in current directory,
        # cd fabric-samples and checkout corresponding version
        echo "===> Checking out v${VERSION} of hyperledger/fabric-samples"
        cd fabric-samples && git checkout v${VERSION}
    else
        echo "===> Cloning hyperledger/fabric-samples repo and checkout v${VERSION}"
        git clone -b master https://github.com/hyperledger/fabric-samples.git && cd fabric-samples && git checkout v${VERSION}
    fi
}

# This will download the .tar.gz
download() {
    local BINARY_FILE=$1
    local URL=$2
    echo "===> Downloading: " "${URL}"
    curl -s -L "${URL}" | tar xz || rc=$?
    if [ -n "$rc" ]; then
        echo "==> There was an error downloading the binary file."
        return 22
    else
        echo "==> Done."
    fi
}

binariesInstall() {
    echo "===> Downloading version ${FABRIC_TAG} platform specific fabric binaries"
    download "${BINARY_FILE}" "https://github.com/hyperledger/fabric/releases/download/v${VERSION}/${BINARY_FILE}"
    if [ $? -eq 22 ]; then
        echo
        echo "------> ${FABRIC_TAG} platform specific fabric binary is not available to download <----"
        echo
        exit
    fi

    echo "===> Downloading version ${CA_TAG} platform specific fabric-ca-client binary"
    download "${CA_BINARY_FILE}" "https://github.com/hyperledger/fabric-ca/releases/download/v${VERSION}/${CA_BINARY_FILE}"
    if [ $? -eq 22 ]; then
        echo
        echo "------> ${CA_TAG} fabric-ca-client binary is not available to download  (Available from 1.1.0-rc1) <----"
        echo
        exit
    fi
}

dockerInstall() {
    command -v docker >& /dev/null
    NODOCKER=$?
    if [ "${NODOCKER}" == 0 ]; then
        echo "===> Pulling fabric Images"
        dockerFabricPull "${FABRIC_TAG}"
        echo "===> Pulling fabric ca Image"
        dockerCaPull "${CA_TAG}"
        echo "===> Pulling thirdparty docker images"
        dockerThirdPartyImagesPull "${THIRDPARTY_TAG}"
        echo
        echo "===> List out hyperledger docker images"
        docker images | grep hyperledger
    else
        echo "========================================================="
        echo "Docker not installed, bypassing download of Fabric images"
        echo "========================================================="
    fi
}

DOCKER=true
SAMPLES=true
BINARIES=true

# Parse commandline args pull out
# version and/or ca-version strings first
if [ -n "$1" ] && [ "${1:0:1}" != "-" ]; then
    VERSION=$1;shift
    if [ -n "$1" ]  && [ "${1:0:1}" != "-" ]; then
        CA_VERSION=$1;shift
        if [ -n  "$1" ] && [ "${1:0:1}" != "-" ]; then
            THIRDPARTY_IMAGE_VERSION=$1;shift
        fi
    fi
fi

# prior to 1.2.0 architecture was determined by uname -m
if [[ $VERSION =~ ^1\.[0-1]\.* ]]; then
    export FABRIC_TAG=${MARCH}-${VERSION}
    export CA_TAG=${MARCH}-${CA_VERSION}
    export THIRDPARTY_TAG=${MARCH}-${THIRDPARTY_IMAGE_VERSION}
else
    # starting with 1.2.0, multi-arch images will be default
    : "${CA_TAG:="$CA_VERSION"}"
    : "${FABRIC_TAG:="$VERSION"}"
    : "${THIRDPARTY_TAG:="$THIRDPARTY_IMAGE_VERSION"}"
fi

BINARY_FILE=hyperledger-fabric-${ARCH}-${VERSION}.tar.gz
CA_BINARY_FILE=hyperledger-fabric-ca-${ARCH}-${CA_VERSION}.tar.gz

# then parse opts
while getopts "h?dsb" opt; do
    case "$opt" in
        h|\?)
            printHelp
            exit 0
            ;;
        d)  DOCKER=false
            ;;
        s)  SAMPLES=false
            ;;
        b)  BINARIES=false
            ;;
    esac
done

if [ "$SAMPLES" == "true" ]; then
    echo
    echo "Installing hyperledger/fabric-samples repo"
    echo
    samplesInstall
fi
if [ "$BINARIES" == "true" ]; then
    echo
    echo "Installing Hyperledger Fabric binaries"
    echo
    binariesInstall
fi
if [ "$DOCKER" == "true" ]; then
    echo
    echo "Installing Hyperledger Fabric docker images"
    echo
    dockerInstall
fi
