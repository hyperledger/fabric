#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# if version not passed in, default to latest released version
VERSION=2.5.10
# if ca version not passed in, default to latest released version
CA_VERSION=1.5.13

REGISTRY=${FABRIC_DOCKER_REGISTRY:-docker.io/hyperledger}

OS=$(uname -s|tr '[:upper:]' '[:lower:]'|sed 's/mingw64_nt.*/windows/')
ARCH=$(uname -m | sed 's/x86_64/amd64/g' | sed 's/aarch64/arm64/g')
PLATFORM=${OS}-${ARCH}

# Fabric < 1.2 uses uname -m for architecture.
MARCH=$(uname -m)

: ${CONTAINER_CLI:="docker"}

printHelp() {
    echo "Usage: bootstrap.sh [version [ca_version]] [options]"
    echo
    echo "options:"
    echo "-h : this help"
    echo "-d : bypass docker image download"
    echo "-s : bypass fabric-samples repo clone"
    echo "-b : bypass download of platform-specific binaries"
    echo
    echo "e.g. bootstrap.sh 2.5.10 1.5.13 -s"
    echo "will download docker images and binaries for Fabric v2.5.10 and Fabric CA v1.5.13"
}

# dockerPull() pulls docker images from fabric and chaincode repositories
# note, if a docker image doesn't exist for a requested release, it will simply
# be skipped, since this script doesn't terminate upon errors.

dockerPull() {
    #three_digit_image_tag is passed in, e.g. "1.4.7"
    three_digit_image_tag=$1
    shift
    #two_digit_image_tag is derived, e.g. "1.4", especially useful as a local tag for two digit references
    two_digit_image_tag=$(echo "$three_digit_image_tag" | cut -d'.' -f1,2)
    while [[ $# -gt 0 ]]
    do
        image_name="$1"
        echo "====>  ${REGISTRY}/fabric-$image_name:$three_digit_image_tag"
        ${CONTAINER_CLI} pull "${REGISTRY}/fabric-$image_name:$three_digit_image_tag"
        ${CONTAINER_CLI} tag "${REGISTRY}/fabric-$image_name:$three_digit_image_tag" "${REGISTRY}/fabric-$image_name"
        ${CONTAINER_CLI} tag "${REGISTRY}/fabric-$image_name:$three_digit_image_tag" "${REGISTRY}/fabric-$image_name:$two_digit_image_tag"
        shift
    done
}

cloneSamplesRepo() {
    # clone (if needed) hyperledger/fabric-samples and checkout corresponding
    # version to the binaries and docker images to be downloaded
    if [ -d test-network ]; then
        # if we are in the fabric-samples repo, checkout corresponding version
        echo "==> Already in fabric-samples repo"
    elif [ -d fabric-samples ]; then
        # if fabric-samples repo already cloned and in current directory,
        # cd fabric-samples
        echo "===> Changing directory to fabric-samples"
        cd fabric-samples
    else
        echo "===> Cloning hyperledger/fabric-samples repo"
        git clone -b main https://github.com/hyperledger/fabric-samples.git && cd fabric-samples
    fi

    if GIT_DIR=.git git rev-parse v${VERSION} >/dev/null 2>&1; then
        echo "===> Checking out v${VERSION} of hyperledger/fabric-samples"
        git checkout -q v${VERSION}
    else
        echo "fabric-samples v${VERSION} does not exist, defaulting to main. fabric-samples main branch is intended to work with recent versions of fabric."
        git checkout -q main
    fi
}

# This will download the .tar.gz
download() {
    local BINARY_FILE=$1
    local URL=$2
    echo "===> Downloading: " "${URL}"
    curl -L --retry 5 --retry-delay 3 "${URL}" | tar xz || rc=$?
    if [ -n "$rc" ]; then
        echo "==> There was an error downloading the binary file."
        return 22
    else
        echo "==> Done."
    fi
}

pullBinaries() {
    echo "===> Downloading version ${FABRIC_TAG} platform specific fabric binaries"
    download "${BINARY_FILE}" "https://github.com/hyperledger/fabric/releases/download/v${VERSION}/${BINARY_FILE}"
    if [ $? -eq 22 ]; then
        echo
        echo "------> ${FABRIC_TAG} platform specific fabric binary is not available to download <----"
        echo
        exit
    fi

    echo "===> Downloading version ${CA_TAG} platform specific fabric-ca-client binary"
    download "${CA_BINARY_FILE}" "https://github.com/hyperledger/fabric-ca/releases/download/v${CA_VERSION}/${CA_BINARY_FILE}"
    if [ $? -eq 22 ]; then
        echo
        echo "------> ${CA_TAG} fabric-ca-client binary is not available to download  (Available from 1.1.0-rc1) <----"
        echo
        exit
    fi
}

pullDockerImages() {
    command -v ${CONTAINER_CLI} >& /dev/null
    NODOCKER=$?
    if [ "${NODOCKER}" == 0 ]; then
        FABRIC_IMAGES=(peer orderer ccenv)
        case "$VERSION" in
        [2-3].*)
            FABRIC_IMAGES+=(baseos)
            shift
            ;;
        esac
        echo "FABRIC_IMAGES:" "${FABRIC_IMAGES[@]}"
        echo "===> Pulling fabric Images"
        dockerPull "${FABRIC_TAG}" "${FABRIC_IMAGES[@]}"
        echo "===> Pulling fabric ca Image"
        CA_IMAGE=(ca)
        dockerPull "${CA_TAG}" "${CA_IMAGE[@]}"
        echo "===> List out hyperledger docker images"
        ${CONTAINER_CLI} images | grep hyperledger
    else
        echo "========================================================="
        echo "${CONTAINER_CLI} not installed, bypassing download of Fabric images"
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

# Prior to fabric 2.5, use amd64 binaries on darwin-arm64
if [[ $VERSION =~ ^2\.[0-4]\.* ]]; then
  PLATFORM=$(echo $PLATFORM | sed 's/darwin-arm64/darwin-amd64/g')
fi

BINARY_FILE=hyperledger-fabric-${PLATFORM}-${VERSION}.tar.gz
CA_BINARY_FILE=hyperledger-fabric-ca-${PLATFORM}-${CA_VERSION}.tar.gz

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
    echo "Clone hyperledger/fabric-samples repo"
    echo
    cloneSamplesRepo
fi
if [ "$BINARIES" == "true" ]; then
    echo
    echo "Pull Hyperledger Fabric binaries"
    echo
    pullBinaries
fi
if [ "$DOCKER" == "true" ]; then
    echo
    echo "Pull Hyperledger Fabric docker images"
    echo
    pullDockerImages
fi
