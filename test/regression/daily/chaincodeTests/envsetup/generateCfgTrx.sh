#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e

CHANNEL_NAME=$1
CHANNEL_COUNT=$2

echo "ch_name : $CHANNEL_NAME"
echo "ch_count : $CHANNEL_COUNT"

: ${CHANNEL_NAME:="mychannel"}
: ${CHANNEL_COUNT:="1"}

export FABRIC_ROOT=$GOPATH/src/github.com/hyperledger/fabric
export E2E_CLI_PATH=$FABRIC_ROOT/examples/e2e_cli/
cp $E2E_CLI_PATH/configtx.yaml $PWD
cp $E2E_CLI_PATH/crypto-config.yaml ./crypto-config.yaml
cp -r $E2E_CLI_PATH/base $PWD/
sed -i 's/e2ecli/envsetup/g' base/peer-base.yaml
export FABRIC_CFG_PATH=$PWD

ARTIFACTS=./channel-artifacts
OS_ARCH=$(echo "$(uname -s)-$(uname -m | sed 's/x86_64/amd64/g')" | awk '{print tolower($0)}')

echo "Channel name - "$CHANNEL_NAME
echo "Total channels - "$CHANNEL_COUNT
echo

CONFIGTXGEN=`which configtxgen || /bin/true`

## Generates Org certs using cryptogen tool
function generateCerts (){
	CRYPTOGEN=$FABRIC_ROOT/release/$OS_ARCH/bin/cryptogen

	if [ -f "$CRYPTOGEN" ]; then
            echo "Using cryptogen -> $CRYPTOGEN"
	else
	    echo "Building cryptogen"
	    make -C $FABRIC_ROOT release-all
	fi

	echo
	echo "**** Generate certificates using cryptogen tool ****"
	$CRYPTOGEN generate --config=./crypto-config.yaml
	echo
}

## Generate orderer genesis block , channel configuration transaction and anchor peer update transactions
function generateChannelArtifacts() {

	CONFIGTXGEN=$FABRIC_ROOT/release/$OS_ARCH/bin/configtxgen
	if [ -f "$CONFIGTXGEN" ]; then
            echo "Using configtxgen -> $CONFIGTXGEN"
	else
	    echo "Building configtxgen"
	    make -C $FABRIC_ROOT release-all
	fi

	echo "Generating genesis block"
	$CONFIGTXGEN -profile TwoOrgsOrdererGenesis -outputBlock $ARTIFACTS/genesis.block

	for (( i=0; $i<$CHANNEL_COUNT; i++))
	do
		echo "Generating channel configuration transaction for channel '$CHANNEL_NAME$i'"
	        $CONFIGTXGEN -profile TwoOrgsChannel -outputCreateChannelTx $ARTIFACTS/channel$i.tx -channelID $CHANNEL_NAME$i
	
		echo "Generating anchor peer update for Org1MSP"
		$CONFIGTXGEN -profile TwoOrgsChannel -outputAnchorPeersUpdate $ARTIFACTS/Org1MSPanchors$i.tx -channelID $CHANNEL_NAME$i -asOrg Org1MSP
	
		echo "Generating anchor peer update for Org2MSP"
		$CONFIGTXGEN -profile TwoOrgsChannel -outputAnchorPeersUpdate $ARTIFACTS/Org2MSPanchors$i.tx -channelID $CHANNEL_NAME$i -asOrg Org2MSP
	done
}

generateCerts
generateChannelArtifacts

echo "######################### DONE ######################"

