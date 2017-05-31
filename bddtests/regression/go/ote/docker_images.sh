#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# Clone fabric git repository
#############################
echo "Fabric Images"
cd $GOPATH/src/github.com/hyperledger/fabric
#FABRIC_COMMIT=$(git log -1 --pretty=format:"%h")
make docker && make native

# Clone fabric-ca git repository
################################
echo "Ca Images"
CA_REPO_NAME=fabric-ca
cd  $GOPATH/src/github.com/hyperledger/
if [ ! -d "$CA_REPO_NAME" ]; then
git clone --depth=1 https://github.com/hyperledger/$CA_REPO_NAME.git
#CA_COMMIT=$(git log -1 --pretty=format:"%h")
fi
cd $CA_REPO_NAME
make docker
echo "List of all Images"
docker images | grep hyperledger
