#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

SDIR="$(dirname ${BASH_SOURCE[0]})"

# Set default variables if they have not been set
: ${INTERACTIVE:=false}
: ${CONFIGTXLATOR_URL:="http://127.0.0.1:7059"}
: ${ORDERER_ADDRESS:=127.0.0.1:7050}

bigMsg() {
	echo
	echo "#####################################################"
	echo "$1"
	echo "#####################################################"
	echo
}

die() {
	bigMsg "FAILURE: $1"
	exit 1
}

pauseIfInteractive() {
	if ${INTERACTIVE} ; then
		echo -en "\nPress enter to continue:"
		read
		echo
	fi
}

verifyEncodeDecodeArgs() {
	if [ "$#" -ne 3 ] ; then
		echo "Requires 3 arguments, got $#"
		return 1
	fi

	if [ ! -e "$2" ] ; then
		echo "Input file '$2' must exist"
		return 1
	fi

	return 0
}

assertFileContainsProto() {
	if [ ! -s "$1" ] ; then
		echo "Unexpected empty file: '$1'"
		return 1
	fi

	if ! (file -b --mime "$1" | grep 'charset=binary' &>/dev/null) ; then
		echo "File '$1' does not appear to be protobuf"
		return 1
	fi

	return 0
}

assertFileContainsJSON() {
	if [ ! -s "$1" ] ; then
		echo "Unexpected empty file: '$1'"
		return 1
	fi

	if ! jq . "$1" &> /dev/null ; then
		echo "File '$1' does not appear to be json"
		return 1
	fi

	return 0
}

decode() {
	verifyEncodeDecodeArgs $@ || die "bad invocation of decode"

	MSG_TYPE=$1
		INPUT_FILE=$2
	OUTPUT_FILE=$3

	echo Executing:
	echo -e "\t" curl -X POST --data-binary @${INPUT_FILE} "$CONFIGTXLATOR_URL/protolator/decode/${MSG_TYPE} > ${OUTPUT_FILE}"

	curl -s -X POST --data-binary @${INPUT_FILE} "$CONFIGTXLATOR_URL/protolator/decode/${MSG_TYPE}" | jq . >  "${OUTPUT_FILE}"
	[ $? -eq 0 ] || die "unable to decode via REST"

	assertFileContainsJSON "${OUTPUT_FILE}" || die "expected JSON output"

	pauseIfInteractive
}

encode() {
	verifyEncodeDecodeArgs $@ || die "with bad invocation of encode"

	MSG_TYPE=$1
		INPUT_FILE=$2
	OUTPUT_FILE=$3

	echo Executing:
	echo -e "\t" curl -X POST --data-binary @${INPUT_FILE} "$CONFIGTXLATOR_URL/protolator/encode/${MSG_TYPE} > ${OUTPUT_FILE}"

	curl -s -X POST --data-binary @${INPUT_FILE} "$CONFIGTXLATOR_URL/protolator/encode/${MSG_TYPE}" >  "${OUTPUT_FILE}"
	[ $? -eq 0 ] || die "unable to encode via REST"

	assertFileContainsProto "${OUTPUT_FILE}" || die "expected protobuf output"

	pauseIfInteractive
}

computeUpdate() {
	if [ "$#" -ne 4 ] ; then
		echo "Requires 4 arguments, got $#"
		exit 1
	fi

	CHANNEL_ID=$1
		ORIGINAL_CONFIG=$2
	UPDATED_CONFIG=$3
	OUTPUT_FILE=$4

	if [ ! -e "${ORIGINAL_CONFIG}" ] ; then
		echo "Input file '${ORIGINAL_CONFIG}' must exist"
		exit 1
	fi

	if [ ! -e "${UPDATED_CONFIG}" ] ; then
		echo "Input file '${UPDATED_CONFIG}' must exist"
		exit 1
	fi

	echo Executing:
	echo -e "\tcurl -X POST -F channel=${CHANNEL_ID} -F original=@${ORIGINAL_CONFIG} -F updated=@${UPDATED_CONFIG} ${CONFIGTXLATOR_URL}/configtxlator/compute/update-from-configs > ${OUTPUT_FILE}"

	curl -s -X POST -F channel="${CHANNEL_ID}" -F "original=@${ORIGINAL_CONFIG}" -F "updated=@${UPDATED_CONFIG}" "${CONFIGTXLATOR_URL}/configtxlator/compute/update-from-configs" > "${OUTPUT_FILE}"
	[ $? -eq 0 ] || die "unable to compute config update via REST"

	assertFileContainsProto "${OUTPUT_FILE}" || die "expected protobuf output"

	pauseIfInteractive
}

fetchConfigBlock() {
	if [ "$#" -ne 2 ] ; then
		echo "Requires 2 arguments, got $#"
		exit 1
	fi

	CHANNEL_ID="$1"
	OUTPUT_FILE="$2"

	echo Executing:
	echo -e "\t$PEER channel fetch config '${OUTPUT_FILE}' -o '${ORDERER_ADDRESS}' -c '${CHANNEL_ID}'"
	$PEER channel fetch config "${OUTPUT_FILE}" -o "${ORDERER_ADDRESS}" -c "${CHANNEL_ID}" &> /dev/null || die "Unable to fetch config block"

	assertFileContainsProto "${OUTPUT_FILE}" || die "expected protobuf output for config block"

	pauseIfInteractive
}

wrapConfigEnvelope() {
	CONFIG_UPDATE_JSON="$1"
	OUTPUT_FILE="$2"

	echo "Wrapping config update in envelope message, like an SDK would."
	echo '{"payload":{"header":{"channel_header":{"channel_id":"'$CHANNEL'", "type":2}},"data":{"config_update":'$(cat ${CONFIG_UPDATE_JSON})'}}}' | jq . > ${OUTPUT_FILE} || die "malformed json"
}

updateConfig() {
	if [ "$#" -ne 2 ] ; then
		echo "Requires 2 arguments, got $#"
		exit 1
	fi

	CHANNEL_ID="$1"
	CONFIGTX="$2"

	echo Executing:
	echo -e "\t$PEER channel update -f '${CONFIGTX}' -c '${CHANNEL_ID}' -o '${ORDERER_ADDRESS}'"
	$PEER channel update -f "${CONFIGTX}" -c "${CHANNEL_ID}" -o "${ORDERER_ADDRESS}" &> /dev/null || die "Error updating channel"

	pauseIfInteractive
}

createChannel() {
	if [ "$#" -ne 2 ] ; then
		echo "Requires 2 arguments, got $#"
		exit 1
	fi

	CHANNEL_ID="$1"
	CONFIGTX="$2"

	echo Executing:
	echo -e "\t$PEER channel create -f '${CONFIGTX}' -c '${CHANNEL_ID}' -o '${ORDERER_ADDRESS}'"
	$PEER channel create -f "${CONFIGTX}" -c "${CHANNEL_ID}" -o "${ORDERER_ADDRESS}" &> /dev/null || die "Error creating channel"

	pauseIfInteractive
}


findConfigtxgen() {
	if hash configtxgen 2>/dev/null ; then
		CONFIGTXGEN=configtxgen
		return 0
	fi

	if [ -e "../../../build/bin/configtxgen" ] ; then
		CONFIGTXGEN="../../../build/bin/configtxgen"
		return 0
	fi

	echo "No configtxgen found, try 'make configtxgen'"
	return 1
}

findConfigtxlator() {
	if hash configtxlator 2>/dev/null ; then
		CONFIGTXGEN=configtxgen
		return 0
	fi

	if [ -e "${SDIR}/../../../build/bin/configtxlator" ] ; then
		CONFIGTXGEN="${SDIR}/../../../build/bin/configtxlator"
		return 0
	fi

	echo "No configtxlator found, try 'make configtxlator'"
	return 1
}

findPeer() {
	if hash peer 2>/dev/null ; then
		PEER=peer
		return 0
	fi

	if [ -e "${SDIR}/../../../build/bin/peer" ] ; then
		PEER="${SDIR}/../../../build/bin/peer"
		return 0
	fi

	echo "No configtxlator found, try 'make configtxlator'"
	return 1
}
