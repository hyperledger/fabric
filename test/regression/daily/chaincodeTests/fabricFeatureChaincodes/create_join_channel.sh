#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set +x
set -e


START_TIME=$(date +%s)
##### GLOBALS ######
CHANNEL_NAME="$1"
CHANNELS="$2"
CHAINCODES="$3"
ENDORSERS="$4"
fun="$5"
LOG_FILE="scripts1/logs.txt"

##### SET DEFAULT VALUES #####
: ${CHANNEL_NAME:="mychannel"}
: ${CHANNELS:="1"}
: ${CHAINCODES:="1"}
: ${ENDORSERS:="4"}
: ${TIMEOUT:="60"}
COUNTER=1
MAX_RETRY=5
ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

echo "Channel name : $CHANNEL_NAME" >>$LOG_FILE
echo "Channels: $CHANNELS" >>$LOG_FILE
echo "Chaincodes : $CHAINCODES" >>$LOG_FILE
echo "Endorsers : $ENDORSERS" >>$LOG_FILE
echo "FUN : $fun" >>$LOG_FILE

verifyResult () {
	if [ $1 -ne 0 ] ; then
		echo "!!!!!!!!!!!!!!! "$2" !!!!!!!!!!!!!!!!" >>$LOG_FILE
                echo "================== ERROR !!! FAILED to execute End-2-End Scenario ==================" >>$LOG_FILE
		echo "!!!!!!!!!!!!!!! "$2" !!!!!!!!!!!!!!!!"
                echo "================== ERROR !!! FAILED to execute End-2-End Scenario =================="
		echo
   		exit 1
	fi
}

setGlobals () {
	if [ $1 -eq 0 -o $1 -eq 1 ] ; then
		export CORE_PEER_LOCALMSPID="Org1MSP"
		export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
		export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
		if [ $1 -eq 0 ]; then
			export CORE_PEER_ADDRESS=peer0.org1.example.com:7051
		else
			export CORE_PEER_ADDRESS=peer1.org1.example.com:7051
		fi
	else
		export CORE_PEER_LOCALMSPID="Org2MSP"
		export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
		export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
		if [ $1 -eq 2 ]; then
			export CORE_PEER_ADDRESS=peer0.org2.example.com:7051
		else
			export CORE_PEER_ADDRESS=peer1.org2.example.com:7051
		fi
	fi
}

createChannel() {
        echo "Inside create channel fun = $fun"
	setGlobals 0
        CH_NUM=$1

        if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer channel create -o orderer.example.com:7050 -c $CHANNEL_NAME$CH_NUM -f ./channel-artifacts/channel$CH_NUM.tx >>$LOG_FILE
	else
		peer channel create -o orderer.example.com:7050 -c $CHANNEL_NAME$CH_NUM -f ./channel-artifacts/channel$CH_NUM.tx --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA >>$LOG_FILE
	fi
	res=$?
	verifyResult $res "Channel creation failed"
	echo "===================== Channel \"$CHANNEL_NAME$CH_NUM\" is created successfully ===================== " >>$LOG_FILE
	echo "===================== Channel \"$CHANNEL_NAME$CH_NUM\" is created successfully ===================== "
	echo
}

updateAnchorPeers() {
        PEER=$1
        CH_NUM=$2
        setGlobals $PEER

        if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer channel create -o orderer.example.com:7050 -c $CHANNEL_NAME$CH_NUM -f ./channel-artifacts/${CORE_PEER_LOCALMSPID}anchors$CH_NUM.tx >>$LOG_FILE
	else
		peer channel create -o orderer.example.com:7050 -c $CHANNEL_NAME$CH_NUM -f ./channel-artifacts/${CORE_PEER_LOCALMSPID}anchors$CH_NUM.tx --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA >>$LOG_FILE
	fi
	res=$?
	verifyResult $res "Anchor peer update failed"
	echo "===================== Anchor peers for org \"$CORE_PEER_LOCALMSPID\" on \"$CHANNEL_NAME\" is updated successfully ===================== " >>$LOG_FILE
	echo "===================== Anchor peers for org \"$CORE_PEER_LOCALMSPID\" on \"$CHANNEL_NAME\" is updated successfully ===================== "
	echo
}

## Sometimes Join takes time hence RETRY atleast for 5 times
joinWithRetry () {
        for (( i=0; $i<$CHANNELS; i++))
          do
		peer channel join -b $CHANNEL_NAME$i.block  >>$LOG_FILE
		res=$?
		if [ $res -ne 0 -a $COUNTER -lt $MAX_RETRY ]; then
			COUNTER=` expr $COUNTER + 1`
			echo "PEER$1 failed to join the channel, Retry after 2 seconds" >> $LOG_FILE
			sleep 2
			joinWithRetry $1
		else
			COUNTER=1
		fi
        	verifyResult $res "After $MAX_RETRY attempts, PEER$ch has failed to Join the Channel"
                echo "===================== PEER$1 joined on the channel \"$CHANNEL_NAME$i\" ===================== " >>$LOG_FILE
                echo "===================== PEER$1 joined on the channel \"$CHANNEL_NAME$i\" successfully ===================== "
                sleep 2
         done
}

joinChannel () {
        for (( peer=0; $peer<$ENDORSERS; peer++))
          do
		setGlobals $peer
		joinWithRetry $peer
		sleep 2
		echo >> $LOG_FILE
	 done
}

validateArgs () {
        if [ -z "${fun}" ]; then
                exit 1
        fi
        if [ "${fun}" == "create" -o "${fun}" == "join" ]; then
                return
        else
                echo "Invalid Argument from vaidateArgs" >>$LOG_FILE
                exit 1
        fi
}

validateArgs
if [ "${fun}" == "create" ]; then
        ## Create channel
        for (( ch=0; $ch<$CHANNELS; ch++))
        do
                createChannel $ch
        done

elif [ "${fun}" == "join" ]; then
        #Join all the peers to all the channels
        joinChannel
        ## Set the anchor peers for each org in the channel
        updateAnchorPeers 0 0
        updateAnchorPeers 2 0
        echo "===================== All GOOD, End-2-End for create_join channel successfully===================== " >>$LOG_FILE
        echo "===================== All GOOD, End-2-End for create_join channel successfully===================== "
        echo
        echo "Total execution time $(($(date +%s)-START_TIME)) secs" >>$LOG_FILE
        echo
        exit 0
else
        printHelp
        exit 1
fi
