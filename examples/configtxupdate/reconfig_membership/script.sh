#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

CDIR=$(dirname $(basename "$0"))

# Defaults
: ${CHANNEL:="example"}
: ${OUTDIR:="example_output"}
mkdir -p ${OUTDIR} || die "could not create output dir ${OUTDIR}"

CONFIG_BLOCK_PB="${OUTDIR}/config_block.pb"
CONFIG_BLOCK_JSON="${OUTDIR}/config_block.json"
CONFIG_JSON="${OUTDIR}/config.json"
CONFIG_PB="${OUTDIR}/config.pb"
UPDATED_CONFIG_JSON="${OUTDIR}/updated_config.json"
UPDATED_CONFIG_PB="${OUTDIR}/updated_config.pb"
CONFIG_UPDATE_PB="${OUTDIR}/config_update.pb"
CONFIG_UPDATE_JSON="${OUTDIR}/config_update.json"
CONFIG_UPDATE_IN_ENVELOPE_PB="${OUTDIR}/config_update_in_envelope.pb"
CONFIG_UPDATE_IN_ENVELOPE_JSON="${OUTDIR}/config_update_in_envelope.json"

CHANNEL_CREATE_TX=${OUTDIR}/channel_create_tx.pb

. ${CDIR}/../common_scripts/common.sh

bigMsg "Beginning config update membership example"

findPeer || die "could not find peer binary"

findConfigtxgen || die "could not find configtxgen binary"

echo -e "Executing:\n\tconfigtxgen -channelID '${CHANNEL}' -outputCreateChannelTx '${CHANNEL_CREATE_TX}' -profile SampleSingleMSPChannel"
${CONFIGTXGEN} -channelID "${CHANNEL}" -outputCreateChannelTx "${CHANNEL_CREATE_TX}" -profile SampleSingleMSPChannel || die "error creating channel create tx"

assertFileContainsProto "${CHANNEL_CREATE_TX}"

bigMsg "Submitting channel create tx to orderer"

createChannel "${CHANNEL}" "${CHANNEL_CREATE_TX}"

bigMsg "Renaming current config block"

echo -e "Executing:\n\tmv '${CHANNEL}.block' '${CONFIG_BLOCK_PB}'"
mv "${CHANNEL}.block" "${CONFIG_BLOCK_PB}"

bigMsg "Decoding current config block"

decode common.Block "${CONFIG_BLOCK_PB}" "${CONFIG_BLOCK_JSON}"

bigMsg "Isolating current config"

echo -e "Executing:\tjq .data.data[0].payload.data.config '${CONFIG_BLOCK_JSON}' > '${CONFIG_JSON}'"
jq .data.data[0].payload.data.config "${CONFIG_BLOCK_JSON}" > "${CONFIG_JSON}" || die "Unable to extract config from config block"

pauseIfInteractive

bigMsg "Generating new config"

jq '. * {"channel_group":{"groups":{"Application":{"groups":{"ExampleOrg": .channel_group.groups.Application.groups.SampleOrg}}}}}'  "${CONFIG_JSON}"  | jq '.channel_group.groups.Application.groups.ExampleOrg.values.MSP.value.config.name = "ExampleOrg"' > "${UPDATED_CONFIG_JSON}"

echo "Used JQ to write new config with 'ExampleOrg' defined to ${UPDATED_CONFIG_JSON}"

pauseIfInteractive

bigMsg "Translating original config to proto"

encode common.Config "${CONFIG_JSON}" "${CONFIG_PB}"

bigMsg "Translating updated config to proto"

encode common.Config "${UPDATED_CONFIG_JSON}" "${UPDATED_CONFIG_PB}"

bigMsg "Computing config update"

computeUpdate "${CHANNEL}" "${CONFIG_PB}" "${UPDATED_CONFIG_PB}" "${CONFIG_UPDATE_PB}"

bigMsg "Decoding config update"

decode common.ConfigUpdate "${CONFIG_UPDATE_PB}" "${CONFIG_UPDATE_JSON}"

bigMsg "Generating config update envelope"

wrapConfigEnvelope "${CONFIG_UPDATE_JSON}" "${CONFIG_UPDATE_IN_ENVELOPE_JSON}"

bigMsg "Encoding config update envelope"

encode common.Envelope "${CONFIG_UPDATE_IN_ENVELOPE_JSON}" "${CONFIG_UPDATE_IN_ENVELOPE_PB}"

bigMsg "Sending config update to channel"

updateConfig ${CHANNEL} "${CONFIG_UPDATE_IN_ENVELOPE_PB}"

bigMsg "Config Update Successful!"
