#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

CDIR=$(dirname $(basename "$0"))

: ${OUTDIR:="example_output"}
mkdir -p ${OUTDIR} || die "could not create output dir ${OUTDIR}"

GENESIS_BLOCK_PB="${OUTDIR}/genesis_block.pb"
GENESIS_BLOCK_JSON="${OUTDIR}/genesis_block.json"
UPDATED_GENESIS_BLOCK_JSON="${OUTDIR}/updated_genesis_block.json"
UPDATED_GENESIS_BLOCK_PB="${OUTDIR}/updated_genesis_block.pb"

. ${CDIR}/../common_scripts/common.sh

findConfigtxgen || die "no configtxgen present"

bigMsg "Creating bootstrap block"

echo -e "Executing:\n\tconfigtxgen -outputBlock '${GENESIS_BLOCK_PB}' -profile SampleSingleMSPSolo"
$CONFIGTXGEN -outputBlock "${GENESIS_BLOCK_PB}" -profile SampleSingleMSPSolo 2>/dev/null || die "Error generating genesis block"

pauseIfInteractive

bigMsg "Decoding genesis block"
decode common.Block "${GENESIS_BLOCK_PB}" "${GENESIS_BLOCK_JSON}"

bigMsg "Updating the genesis config"
ORIGINAL_BATCHSIZE=$(jq ".data.data[0].payload.data.config.channel_group.groups.Orderer.values.BatchSize.value.max_message_count" ${GENESIS_BLOCK_JSON})
NEW_BATCHSIZE=$(( ${ORIGINAL_BATCHSIZE} + 1 ))
echo "Updating batch size from ${ORIGINAL_BATCHSIZE} to ${NEW_BATCHSIZE}."
echo -e "Executing\n\tjq '.data.data[0].payload.data.config.channel_group.groups.Orderer.values.BatchSize.value.max_message_count = ${NEW_BATCHSIZE}' '${GENESIS_BLOCK_JSON}' > '${UPDATED_GENESIS_BLOCK_JSON}'"
jq ".data.data[0].payload.data.config.channel_group.groups.Orderer.values.BatchSize.value.max_message_count = ${NEW_BATCHSIZE}" "${GENESIS_BLOCK_JSON}" > "${UPDATED_GENESIS_BLOCK_JSON}"

pauseIfInteractive

bigMsg "Re-encoding the updated genesis block"

encode common.Block "${UPDATED_GENESIS_BLOCK_JSON}" "${UPDATED_GENESIS_BLOCK_PB}"

bigMsg "Bootstrapping edit complete"
