#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


START_TIME=$(date +%s)

##### GLOBALS ######
CHANNEL_NAME="$1"
CHANNELS="$2"
CHAINCODES="$3"
ENDORSERS="$4"
fun="$5"

##### SET DEFAULT VALUES #####
: ${CHANNEL_NAME:="mychannel"}
: ${CHANNELS:="1"}
: ${CHAINCODES:="1"}
: ${ENDORSERS:="4"}
: ${TIMEOUT:="60"}
COUNTER=1
MAX_RETRY=5
CHAINCODE_NAME="myccex02"
LOG_FILE="scripts1/logs.txt"
TEMP_LOG_FILE="scripts1/temp_logs.txt"
ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

verifyResult () {
	if [ $1 -ne 0 ] ; then
		echo "!!!!!!!!!!!!!!! "$2" !!!!!!!!!!!!!!!!" >>$LOG_FILE
                echo "================== ERROR !!! FAILED to execute End-2-End Scenario example02 ==================" >>$LOG_FILE
		echo "!!!!!!!!!!!!!!! "$2" !!!!!!!!!!!!!!!!"
                echo "================== ERROR !!! FAILED to execute End-2-End Scenario example02 =================="
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
			export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
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

installChaincode () {
        for (( i=0; $i<$ENDORSERS; i++))
        do
                for (( ch=0; $ch<$CHAINCODES; ch++))
                do
                        PEER=$i
                        setGlobals $PEER
                        peer chaincode install -n $CHAINCODE_NAME$ch -v 1 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 >>$LOG_FILE
                        res=$?
                        verifyResult $res "Chaincode '$CHAINCODE_NAME$ch' installation on remote peer PEER$PEER has Failed"
                        echo "===================== Chaincode '$CHAINCODE_NAME$ch' is installed on PEER$PEER successfully===================== " >>$LOG_FILE
                        echo "===================== Chaincode '$CHAINCODE_NAME$ch' is installed on PEER$PEER successfully===================== "
                        echo
                done
        done
}

instantiateChaincode () {
        PEER=$1
        for (( i=0; $i<$CHANNELS; i++))
        do
                for (( ch=0; $ch<$CHAINCODES; ch++))
                do
                        setGlobals $PEER
                        if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
                                peer chaincode instantiate -o orderer.example.com:7050 -C $CHANNEL_NAME$i -n $CHAINCODE_NAME$ch -v 1 -c '{"Args":["init","a","1000","b","2000"]}' -P "OR        ('Org0MSP.member','Org1MSP.member')" >>$LOG_FILE
	                else
		                peer chaincode instantiate -o orderer.example.com:7050 --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$i -n $CHAINCODE_NAME$ch -v 1 -c '{"Args":["init","a","1000","b","2000"]}' -P "OR	('Org1MSP.member','Org2MSP.member')" >>$LOG_FILE
	                fi
                        res=$?
                        verifyResult $res "Chaincode '$CHAINCODE_NAME$ch' instantiation on PEER$PEER on channel '$CHANNEL_NAME$i' failed"
                        echo "===================== Chaincode '$CHAINCODE_NAME$ch' Instantiation on PEER$PEER on '$CHANNEL_NAME$i' is successful ===================== ">>$LOG_FILE
                        echo "===================== Chaincode '$CHAINCODE_NAME$ch' Instantiation on PEER$PEER on '$CHANNEL_NAME$i' is successful ===================== "
                        echo
                done
        done
}

chaincodeInvoke () {
        local CH_NUM=$1
        local CHAIN_NUM=$2
        local PEER=$3
        setGlobals $PEER
        if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
             peer chaincode invoke -o ordere.example.com:7050  -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["invoke","a","b","10"]}' >>$LOG_FILE
	else
	     peer chaincode invoke -o orderer.example.com:7050  --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["invoke","a","b","10"]}' >>$LOG_FILE
	fi
        res=$?
        verifyResult $res "Invoke execution on PEER$PEER failed "
        echo "===================== Invoke transaction on PEER$PEER on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ===================== ">>$LOG_FILE
        echo "===================== Invoke transaction on PEER$PEER on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ===================== "
        echo
}


chaincodeQuery () {
        local channel_num=$1
        local chain_num=$2
        local peer=$3
        local res=$4
        echo "===================== Querying on PEER$peer on $CHANNEL_NAME$channel_num/$CHAINCODE_NAME$chain_num... ===================== "
        local rc=1
        local starttime=$(date +%s)

        # continue to poll
        # we either get a successful response, or reach TIMEOUT
        while test "$(($(date +%s)-starttime))" -lt "$TIMEOUT" -a $rc -ne 0
        do
                sleep 3
                echo "Attempting to Query PEER$peer ...$(($(date +%s)-starttime)) secs" >>$LOG_FILE
                peer chaincode query -C $CHANNEL_NAME$channel_num -n $CHAINCODE_NAME$chain_num -c '{"Args":["query","a"]}' >$TEMP_LOG_FILE
                cat $TEMP_LOG_FILE >>$LOG_FILE
                test $? -eq 0 && VALUE=$(cat $TEMP_LOG_FILE | awk '/Query Result/ {print $NF}')
                test "$VALUE" = "$res" && let rc=0
        done
        echo
        if test $rc -eq 0 ; then
                echo "===================== Query on PEER$peer on $CHANNEL_NAME$channel_num/$CHAINCODE_NAME$chain_num is successful ===================== " >>$LOG_FILE
                echo "===================== Query on PEER$peer on $CHANNEL_NAME$channel_num/$CHAINCODE_NAME$chain_num is successful ===================== "
                echo
        else
                echo "!!!!!!!!!!!!!!! Query result on PEER$peer is INVALID !!!!!!!!!!!!!!!!" >>$LOG_FILE
                echo "================== ERROR !!! FAILED to execute End-2-End Scenario ==================">>$LOG_FILE
                echo "!!!!!!!!!!!!!!! Query result on PEER$peer is INVALID !!!!!!!!!!!!!!!!"
                echo "================== ERROR !!! FAILED to execute End-2-End Scenario =================="
                echo
                echo "Total execution time $(($(date +%s)-START_TIME)) secs">>$LOG_FILE
                echo
                exit 1
        fi
}

validateArgs () {
        echo "Inside Validate Args fun = $fun"
        if [ -z "${fun}" ]; then
                exit 1
        fi
        if [ "${fun}" = "install" -o "${fun}" == "instantiate" -o "${fun}" == "invokeQuery" ]; then
                return
        else
                echo "Invalid Argument in validateArgs from e2e_test_example02">>$LOG_FILE
                echo "Invalid Argument in validateArgs from e2e_test_example02"
                exit 1
        fi
}

validateArgs
if [ "${fun}" = "install" ]; then
## Install chaincode on all peers
        installChaincode

elif [ "${fun}" == "instantiate" ]; then
#Instantiate chaincode on Peer2/Org1
        echo "Instantiating chaincode on all channels on PEER0 ..."
        instantiateChaincode 2

elif [ "${fun}" == "invokeQuery" ]; then
#Invoke/Query on all chaincodes on all channels
        echo "send Invokes/Queries on all channels ..."
        for (( ch=0; $ch<$CHANNELS; ch++))
        do
                for (( chain=0; $chain<$CHAINCODES; chain++))
                do
                        AVAL=1000
                        for (( peer_number=0;peer_number<4;peer_number++))
                        do
                                setGlobals "$peer_number"
                                chaincodeQuery $ch $chain $peer_number "$AVAL"
                                chaincodeInvoke $ch $chain $peer_number
                                AVAL=` expr $AVAL - 10 `
                                chaincodeQuery $ch $chain $peer_number "$AVAL"
                        done
                done
        done
        echo "===================== End-2-End for chaincode example02 completed successfully ===================== "  >>$LOG_FILE
        echo "===================== End-2-End for chaincode example02 completed successfully ===================== "
        echo
        echo "Total execution time $(($(date +%s)-START_TIME)) secs" >>$LOG_FILE
        echo
        exit 0
else
        exit 1
fi
