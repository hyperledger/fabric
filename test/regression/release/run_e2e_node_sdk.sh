#!/bin/bash

#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#     

rm -rf $GOPATH/src/github.com/hyperledger/fabric-sdk-node

WD="$GOPATH/src/github.com/hyperledger/fabric-sdk-node"
SDK_REPO_NAME=fabric-sdk-node
git clone https://github.com/hyperledger/$SDK_REPO_NAME $WD
cd $WD
git checkout tags/v1.0.0
cd test/fixtures
docker rm -f "$(docker ps -aq)" || true
docker-compose up >> node_dockerlogfile.log 2>&1 &
sleep 10
docker ps -a
cd ../.. && npm install
npm config set prefix ~/npm && npm install -g gulp && npm install -g istanbul
gulp || true
gulp ca || true
rm -rf node_modules/fabric-ca-client && npm install
gulp test

docker rm -f "$(docker ps -aq)" || true
