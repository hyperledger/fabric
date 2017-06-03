#!/bin/bash

CHANNEL=testchainid

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

bigMsg "Beginning config update example"

PEER=../../../build/bin/peer

bigMsg "Fetching current config block"

$PEER channel fetch config config_block.proto -o 127.0.0.1:7050 -c $CHANNEL || die "Unable to fetch config block"

bigMsg "Decoding current config block"

curl -X POST --data-binary @config_block.proto http://127.0.0.1:7059/protolator/decode/common.Block > config_block.json || die "Unable to decode config block"

bigMsg "Isolating current config"

jq .data.data[0].payload.data.config config_block.json > config.json || die "Unable to extract config from config block"

bigMsg "Generating new config"

OLD_BATCH_SIZE=$(jq ".channel_group.groups.Orderer.values.BatchSize.value.max_message_count" config.json)
NEW_BATCH_SIZE=$(($OLD_BATCH_SIZE+1))

jq ".channel_group.groups.Orderer.values.BatchSize.value.max_message_count = $NEW_BATCH_SIZE" config.json  > updated_config.json || die "Error updating batch size"

bigMsg "Translating original config to proto"

curl -X POST --data-binary @config.json http://127.0.0.1:7059/protolator/encode/common.Config > config.proto || die "Error re-encoding config"

bigMsg "Translating updated config to proto"

curl -X POST --data-binary @updated_config.json http://127.0.0.1:7059/protolator/encode/common.Config > updated_config.proto || die "Error re-encoding updated config"

bigMsg "Computing config update"

curl -X POST -F original=@config.proto -F updated=@updated_config.proto http://127.0.0.1:7059/configtxlator/compute/update-from-configs -F channel=$CHANNEL > config_update.proto || die "Error computing config update"

bigMsg "Decoding config update"

curl -X POST --data-binary @config_update.proto http://127.0.0.1:7059/protolator/decode/common.ConfigUpdate > config_update.json || die "Error decoding config update"

bigMsg "Generating config update envelope"

echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CHANNEL'", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' > config_update_as_envelope.json || die "Error creating config update envelope"

bigMsg "Encoding config update envelope"

curl -X POST --data-binary @config_update_as_envelope.json http://127.0.0.1:7059/protolator/encode/common.Envelope > config_update_as_envelope.proto || die "Error converting envelope to proto"

bigMsg "Sending config update to channel"

$PEER channel update -f config_update_as_envelope.proto -c $CHANNEL -o 127.0.0.1:7050 || die "Error updating channel"

bigMsg "Config Update Successful!"
