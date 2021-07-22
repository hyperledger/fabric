#!/bin/bash

# SPDX-License-Identifier: Apache-2.0

docker tag hyperledger/fabric-tools harrymknight/fabric-tools
docker tag hyperledger/fabric-orderer harrymknight/fabric-orderer
docker tag hyperledger/fabric-peer harrymknight/fabric-peer
docker push harrymknight/fabric-tools
docker push harrymknight/fabric-orderer
docker push harrymknight/fabric-peer
echo "Pushed images"