#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# if version not passed in, default to latest released version
export VERSION=1.1.0
# if ca version not passed in, default to latest released version
export CA_VERSION=$VERSION
# current version of thirdparty images (couchdb, kafka and zookeeper) released
export THIRDPARTY_IMAGE_VERSION=0.4.6
export ARCH=$(echo "$(uname -s|tr '[:upper:]' '[:lower:]'|sed 's/mingw64_nt.*/windows/')-$(uname -m | sed 's/x86_64/amd64/g')" | awk '{print tolower($0)}')
#Set MARCH variable i.e ppc64le,s390x,x86_64,i386
MARCH=`uname -m`

printHelp() {
  echo "Usage: bootstrap.sh [<version>] [<ca_version>] [-d -s -b]"
  echo
  echo "-d - bypass docker image download"
  echo "-s - bypass fabric-samples repo clone"
  echo "-b - bypass download of platform-specific binaries"
  echo
  echo "e.g. bootstrap.sh 1.1.1 -s"
  echo "would download docker images and binaries for version 1.1.1"
}

dockerFabricPull() {
  local FABRIC_TAG=$1
  for IMAGES in peer orderer ccenv javaenv tools; do
      echo "==> FABRIC IMAGE: $IMAGES"
      echo
      docker pull hyperledger/fabric-$IMAGES:$FABRIC_TAG
      docker tag hyperledger/fabric-$IMAGES:$FABRIC_TAG hyperledger/fabric-$IMAGES
  done
}

dockerThirdPartyImagesPull() {
  local THIRDPARTY_TAG=$1
  for IMAGES in couchdb kafka zookeeper; do
      echo "==> THIRDPARTY DOCKER IMAGE: $IMAGES"
      echo
      docker pull hyperledger/fabric-$IMAGES:$THIRDPARTY_TAG
      docker tag hyperledger/fabric-$IMAGES:$THIRDPARTY_TAG hyperledger/fabric-$IMAGES
  done
}

dockerCaPull() {
      local CA_TAG=$1
      echo "==> FABRIC CA IMAGE"
      echo
      docker pull hyperledger/fabric-ca:$CA_TAG
      docker tag hyperledger/fabric-ca:$CA_TAG hyperledger/fabric-ca
}

: ${CA_TAG:="$MARCH-$CA_VERSION"}
: ${FABRIC_TAG:="$MARCH-$VERSION"}
: ${THIRDPARTY_TAG:="$MARCH-$THIRDPARTY_IMAGE_VERSION"}

samplesInstall() {
  # clone (if needed) hyperledger/fabric-samples and checkout corresponding
  # version to the binaries and docker images to be downloaded
  if [ -d first-network ]; then
    # if we are in the fabric-samples repo, checkout corresponding version
    echo "===> Checking out v${VERSION} branch of hyperledger/fabric-samples"
    git checkout v${VERSION}
  elif [ -d fabric-samples ]; then
    # if fabric-samples repo already cloned and in current directory,
    # cd fabric-samples and checkout corresponding version
    echo "===> Checking out v${VERSION} branch of hyperledger/fabric-samples"
    cd fabric-samples && git checkout v${VERSION}
  else
    echo "===> Cloning hyperledger/fabric-samples repo and checkout v${VERSION}"
    git clone -b master https://github.com/hyperledger/fabric-samples.git && cd fabric-samples && git checkout v${VERSION}
  fi
}

binariesInstall() {
  echo "===> Downloading version ${FABRIC_TAG} platform specific fabric binaries"
  curl https://nexus.hyperledger.org/content/repositories/releases/org/hyperledger/fabric/hyperledger-fabric/${ARCH}-${VERSION}/hyperledger-fabric-${ARCH}-${VERSION}.tar.gz | tar xz

  echo "===> Downloading version ${CA_TAG} platform specific fabric-ca-client binary"
  curl https://nexus.hyperledger.org/content/repositories/releases/org/hyperledger/fabric-ca/hyperledger-fabric-ca/${ARCH}-${VERSION}/hyperledger-fabric-ca-${ARCH}-${VERSION}.tar.gz | tar xz
  if [ $? != 0 ]; then
     echo
     echo "------> ${CA_TAG} fabric-ca-client binary is not available to download  (Avaialble from 1.1.0-rc1) <----"
     echo
   fi
}

dockerInstall() {
  which docker >& /dev/null
  NODOCKER=$?
  if [ "${NODOCKER}" == 0 ]; then
	  echo "===> Pulling fabric Images"
	  dockerFabricPull ${FABRIC_TAG}
	  echo "===> Pulling fabric ca Image"
	  dockerCaPull ${CA_TAG}
	  echo "===> Pulling thirdparty docker images"
	  dockerThirdPartyImagesPull ${THIRDPARTY_TAG}
	  echo
	  echo "===> List out hyperledger docker images"
	  docker images | grep hyperledger*
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
if echo $1 | grep -q '\d'; then
  VERSION=$1;shift
  if echo $1 | grep -q '\d'; then
    CA_VERSION=$1;shift
  fi
fi

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
