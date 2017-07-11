#!/bin/bash

#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -o pipefail

CWD=$GOPATH/src/github.com/hyperledger/fabric
cd $CWD

VERSION=`cat Makefile | grep BASE_VERSION | awk '{print $3}' | head -n1`
echo "===>Release_VERSION: $VERSION"

makeCleanAll() {

  make clean-all
  echo "clean-all from fabric repository"
 }

# make native
makeNative() {

  make native
  for binary in chaintool configtxgen configtxlator cryptogen orderer peer; do
  	if [ ! -f $CWD/build/bin/$binary ] ; then
     	   echo " ====> ERROR !!! $binary is not available"
     	   echo
           exit 1
        fi

           echo " ====> PASS !!! $binary is available"
  done
}

# Build peer, orderer, configtxgen, cryptogen and configtxlator
makeBinary() {

   make clean-all
   make peer && make orderer && make configtxgen && make cryptogen && make configtxlator
   for binary in peer orderer configtxgen cryptogen configtxlator; do
         if [ ! -f $CWD/build/bin/$binary ] ; then
     	   echo " ====> ERROR !!! $binary is not available"
     	   echo
           exit 1
        fi

           echo " ====> PASS !!! $binary is available"
   done
}

# Create tar files for each platform
makeDistAll() {

   make clean-all
   make dist-all
   for dist in linux-amd64 windows-amd64 darwin-amd64 linux-ppc64le linux-s390x; do
        if [ ! -d $CWD/release/$dist ] ; then
     	   echo " ====> ERROR !!! $dist is not available"
     	   echo
           exit 1
        fi

           echo " ====> PASS !!! $dist is available"
done
}

# Create docker images
makeDocker() {
    make docker-clean
    make docker
        if [ $? -ne 0 ] ; then
           echo " ===> ERROR !!! Docker Images are not available"
           echo
           exit 1
        fi
           echo " ===> PASS !!! Docker Images are available"
}

# Verify the version built in peer and configtxgen binaries
makeVersion() {
    make docker-clean
    make release
    cd release/linux-amd64/bin
    ./peer --version > peer.txt
    Pversion=$(grep -v "2017" peer.txt | grep Version: | awk '{print $2}' | head -n1)
        if [ "$Pversion" != "$VERSION" ]; then
           echo " ===> ERROR !!! Peer Version check failed"
           echo
        fi
   ./configtxgen --version > configtxgen.txt
   Configtxgen=$(grep -v "2017" configtxgen.txt | grep Version: | awk '{print $2}' | head -n1)
        if [ "$Configtxgen" != "$VERSION" ]; then
           echo "====> ERROR !!! configtxgen Version check failed:"
           echo
           exit 1
        fi
           echo "====> PASS !!! Configtxgen version verified:"
}

$1
