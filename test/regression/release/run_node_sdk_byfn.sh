#!/bin/bash -eu

#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# RUN END-to-END Test
#####################
rm -rf ${WORKSPACE}/gopath/src/github.com/hyperledger/fabric-sdk-node

WD="${WORKSPACE}/gopath/src/github.com/hyperledger/fabric-sdk-node"
SDK_REPO_NAME=fabric-sdk-node
git clone ssh://hyperledger-jobbuilder@gerrit.hyperledger.org:29418/$SDK_REPO_NAME $WD
cd $WD
git checkout tags/v1.0.0
NODE_SDK_COMMIT=$(git log -1 --pretty=format:"%h")

echo "FABRIC NODE SDK COMMIT ========> " $NODE_SDK_COMMIT >> ${WORKSPACE}/gopath/src/github.com/hyperledger/commit_history.log

cd test/fixtures
cat docker-compose.yaml > docker-compose.log
docker-compose up >> dockerlogfile.log 2>&1 &
sleep 10
docker ps -a
cd ../.. && npm install
npm config set prefix ~/npm && npm install -g gulp && npm install -g istanbul
gulp && gulp ca
rm -rf node_modules/fabric-ca-client && npm install
node test/integration/e2e.js


