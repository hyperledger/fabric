#!/bin/bash

die() {
    echo "$1"
    exit 1
}

bigMsg() {
    echo
    echo "####################################################################################"
    echo "$1"
    echo "####################################################################################"
    echo
}

# set -x

bigMsg "Beginning bootstrap editing example"

CONFIGTXGEN=../../../build/bin/configtxgen

bigMsg "Creating bootstrap block"

$CONFIGTXGEN -outputBlock genesis_block.proto || die "Error generating genesis block"

bigMsg "Decoding genesis block"

curl -X POST --data-binary @genesis_block.proto http://127.0.0.1:7059/protolator/decode/common.Block > genesis_block.json || die "Error decoding genesis block"

bigMsg "Updating genesis config"

jq ".data.data[0].payload.data.config.channel_group.groups.Orderer.values.BatchSize.value.maxMessageCount" genesis_block.json > updated_genesis_block.json

bigMsg "Re-encoding the updated genesis block"

curl -X POST --data-binary @updated_genesis_block.json http://127.0.0.1:7059/protolator/encode/common.Block > updated_genesis_block.proto

bigMsg "Bootstrapping edit complete"
