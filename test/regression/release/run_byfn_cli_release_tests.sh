#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# RUN BYFN Test
#####################

CH_NAME="$1"
rm -rf ${GOPATH}/src/github.com/hyperledger/fabric-samples

WD="${GOPATH}/src/github.com/hyperledger/fabric-samples"
REPO_NAME=fabric-samples

git clone ssh://hyperledger-jobbuilder@gerrit.hyperledger.org:29418/$REPO_NAME $WD
cd $WD

curl -L https://raw.githubusercontent.com/hyperledger/fabric/master/scripts/bootstrap-1.0.0.sh -o bootstrap-1.0.0.sh
chmod +x bootstrap-1.0.0.sh
./bootstrap-1.0.0.sh

cd $WD/first-network
export PATH=$WD/bin:$PATH
echo y | ./byfn.sh -m down

     if [ -z "${CH_NAME}" ]; then
          echo "Generating artifacts for default channel"
          echo y | ./byfn.sh -m generate
	  echo "setting to default channel 'mychannel'"
          echo y | ./byfn.sh -m up -t 10
          echo
      else
          echo "Generate artifacts for custom Channel"
          echo y | ./byfn.sh -m generate -c $CH_NAME
          echo "Setting to non-default channel $CH_NAME"
          echo y | ./byfn.sh -m up -c $CH_NAME -t 10
          echo
     fi
echo y | ./byfn.sh -m down
