#!/bin/bash

set -e

CHANNEL_NAME=$1
if [ -z "$1" ]; then
	echo "Setting channel to default name 'mychannel'"
	CHANNEL_NAME="mychannel"
fi

export FABRIC_ROOT=$PWD/../..
export FABRIC_CFG_PATH=$PWD

echo "Channel name - "$CHANNEL_NAME
echo

CONFIGTXGEN=`which configtxgen || /bin/true`

if [ "$CONFIGTXGEN" == "" ]; then
    echo "Building configtxgen"
    make -C $FABRIC_ROOT configtxgen
    CONFIGTXGEN=$FABRIC_ROOT/build/bin/configtxgen
else
    echo "Using configtxgen -> $CONFIGTXGEN"
fi

echo "Generating genesis block"
$CONFIGTXGEN -profile TwoOrgsOrdererGenesis -outputBlock crypto/orderer/orderer.block

echo "Generating channel configuration transaction"
$CONFIGTXGEN -profile TwoOrgsChannel -outputCreateChannelTx crypto/orderer/channel.tx -channelID $CHANNEL_NAME

echo "Generating anchor peer update for Org0MSP"
$CONFIGTXGEN -profile TwoOrgsChannel -outputAnchorPeersUpdate crypto/orderer/Org0MSPanchors.tx -channelID $CHANNEL_NAME -asOrg Org0MSP

echo "Generating anchor peer update for Org1MSP"
$CONFIGTXGEN -profile TwoOrgsChannel -outputAnchorPeersUpdate crypto/orderer/Org1MSPanchors.tx -channelID $CHANNEL_NAME -asOrg Org1MSP
