#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

##### GLOBALS ######
CHANNEL_NAME="$1"
CHANNELS="$2"
CHAINCODES="$3"
ENDORSERS="$4"
RUN_TIME_HOURS="$5"
FN_NAME="$6"

START_COUNT=100
END_COUNT=1000
INTERVAL=100
SLEEP_TIME=10

##### SET DEFAULT VALUES #####
: ${CHANNEL_NAME:="mychannel"}
: ${CHANNELS:="1"}
: ${CHAINCODES:="1"}
: ${ENDORSERS:="4"}
: ${TIMEOUT:="10"}
: ${RUN_TIME_HOURS:="1"}
COUNTER=1
MAX_RETRY=5
CHAINCODE_NAME="auctioncc"
LOG_FILE="$GOPATH/src/github.com/hyperledger/fabric/test/envsetup/logs/auction_logs.log"
TEMP_LOG_FILE="temp_auction_logs.log"
ORDERER_IP=orderer.example.com:7050
LOG_LEVEL="error"

ORDERER_CA=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

function wait() {
	printf "\nWait for $1 secs\n"
	sleep $1
}

verifyResult() {
	if [ $1 -ne 0 ]; then
		echo "================== ERROR !!! FAILED to execute End-2-End Scenario Auction ==================" >>$LOG_FILE
		echo "!!!!!!!!!!!!!!! "$2" !!!!!!!!!!!!!!!!" >>$LOG_FILE
		echo "!!!!!!!!!!!!!!! "$2" !!!!!!!!!!!!!!!!"
		echo "================== ERROR !!! FAILED to execute End-2-End Scenario Auction=================="
		exit 1
	fi
}

function setGlobals() {

	if [ $1 -eq 0 -o $1 -eq 1 ]; then
		CORE_PEER_LOCALMSPID="Org1MSP"
		CORE_PEER_TLS_ROOTCERT_FILE=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
		CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
		if [ $1 -eq 0 ]; then
			CORE_PEER_ADDRESS=peer0.org1.example.com:7051
		else
			CORE_PEER_ADDRESS=peer1.org1.example.com:7051
			CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
		fi
	else
		CORE_PEER_LOCALMSPID="Org2MSP"
		CORE_PEER_TLS_ROOTCERT_FILE=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
		CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
		if [ $1 -eq 2 ]; then
			CORE_PEER_ADDRESS=peer0.org2.example.com:7051
		else
			CORE_PEER_ADDRESS=peer1.org2.example.com:7051
		fi
	fi
}

createChannel() {
	setGlobals 0
	CH_NUM=$1

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer channel create -o $ORDERER_IP -c $CHANNEL_NAME$CH_NUM -f $GOPATH/src/github.com/hyperledger/fabric/peer/channel-artifacts/channel$CH_NUM.tx -t 10 >&$LOG_FILE
		wait 5
	else
		peer channel create -o $ORDERER_IP -c $CHANNEL_NAME$CH_NUM -f $GOPATH/src/github.com/hyperledger/fabric/peer/channel-artifacts/channel$CH_NUM.tx --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -t 10 >&$LOG_FILE
		wait 5
	fi

	res=$?
	verifyResult $res "Channel creation failed"
	echo "==== Channel \"$CHANNEL_NAME\" is created successfully ==== " >>$LOG_FILE
	echo
}

updateAnchorPeers() {
	PEER=$1
	CH_NUM=$2
	setGlobals $PEER

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer channel create -o $ORDERER_IP -c $CHANNEL_NAME$CH_NUM -f $GOPATH/src/github.com/hyperledger/fabric/peer/channel-artifacts/${CORE_PEER_LOCALMSPID}anchors$CH_NUM.tx >>$LOG_FILE
	else
		peer channel create -o $ORDERER_IP -c $CHANNEL_NAME$CH_NUM -f $GOPATH/src/github.com/hyperledger/fabric/peer/channel-artifacts/${CORE_PEER_LOCALMSPID}anchors$CH_NUM.tx --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA >>$LOG_FILE
	fi

	res=$?
	verifyResult $res "Anchor peer update failed"
	echo "==== Anchor peers for org \"$CORE_PEER_LOCALMSPID\" on \"$CHANNEL_NAME\" is updated successfully ==== " >>$LOG_FILE
	echo
}

## Sometimes Join takes time hence RETRY atleast for 5 times
joinWithRetry() {
	for ((i = 0; $i < $CHANNELS; i++)); do
		peer channel join -b $CHANNEL_NAME$i.block >>$LOG_FILE
		res=$?

		if [ $res -ne 0 -a $COUNTER -lt $MAX_RETRY ]; then
			COUNTER=$(expr $COUNTER + 1)
			echo "PEER$1 failed to join the channel, Retry after 2 seconds" >>$LOG_FILE
			wait 2
			joinWithRetry $1
		else
			COUNTER=1
		fi
		verifyResult $res "After $MAX_RETRY attempts, PEER$ch has failed to Join the Channel"
		echo "==== PEER$1 joined on the channel \"$CHANNEL_NAME$i\" ==== " >>$LOG_FILE
		wait 2
	done
}

joinChannel() {
	for ((peer = 0; $peer < $ENDORSERS; peer++)); do
		setGlobals $peer
		joinWithRetry $peer
		wait 2
		echo
	done
}

installChaincode() {
	for ((i = 0; $i < $ENDORSERS; i++)); do
		for ((ch = 0; $ch < $CHAINCODES; ch++)); do
			PEER=$i
			setGlobals $PEER
			peer chaincode install -n $CHAINCODE_NAME$ch -v 1 -p github.com/hyperledger/fabric/test/chaincodes/AuctionApp >>$LOG_FILE

			res=$?
			verifyResult $res "Chaincode '$CHAINCODE_NAME$ch' installation on remote peer PEER$PEER has Failed"
			echo "==== Chaincode '$CHAINCODE_NAME$ch' is installed on remote peer PEER$PEER ==== " >>$LOG_FILE
			echo
		done
	done
}

instantiateChaincode() {
	PEER=$1
	setGlobals $PEER

	for ((i = 0; $i < $CHANNELS; i++)); do
		for ((ch = 0; $ch < $CHAINCODES; ch++)); do
			if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
				peer chaincode instantiate -o $ORDERER_IP -C $CHANNEL_NAME$i -n $CHAINCODE_NAME$ch -v 1 -c '{"Args":["init"]}' -P "OR ('Org1MSP.member','Org2MSP.member')" >>$LOG_FILE
			else
				peer chaincode instantiate -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$i -n $CHAINCODE_NAME$ch -v 1 -c '{"Args":["init"]}' -P "OR ('Org1MSP.member','Org2MSP.member')" >>$LOG_FILE
			fi

			res=$?
			verifyResult $res "Chaincode '$CHAINCODE_NAME$ch' instantiation on PEER$PEER on channel '$CHANNEL_NAME$i' failed"
			echo "==== Chaincode '$CHAINCODE_NAME$ch' Instantiation on PEER$PEER on channel '$CHANNEL_NAME$i' is successful ==== " >>$LOG_FILE
			echo
		done
	done
	wait 10
}

function postUsers() {
	local CH_NUM=$1
	local CHAIN_NUM=$2

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","100", "USER", "Ashley Hart", "TRD",  "Morrisville Parkway, #216, Morrisville, NC 27560", "9198063535", "ashley@itpeople.com", "SUNTRUST", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","200", "USER", "Sotheby", "AH",  "One Picadally Circus , #216, London, UK ", "9198063535", "admin@sotheby.com", "Standard Chartered", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","300", "USER", "Barry Smith", "TRD",  "155 Regency Parkway, #111, Cary, 27518 ", "9198063535", "barry@us.ibm.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","400", "USER", "Cindy Patterson", "TRD",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "cpatterson@hotmail.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","500", "USER", "Tamara Haskins", "TRD",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "tamara@yahoo.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","600", "USER", "NY Life", "INS",  "155 Broadway, New York, NY, USA ", "9058063535", "barry@nyl.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","700", "USER", "J B Hunt", "SHP",  "One Johnny Blvd, Rogers, AR, USA ", "9058063535", "jess@jbhunt.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","800", "USER", "R&R Trading", "AH",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "larry@rr.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","900", "USER", "Gregory Huffman", "TRD",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "tamara@yahoo.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","1000", "USER", "Texas Life", "INS",  "155 Broadway, New York, NY, USA ", "9058063535", "barry@nyl.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","1100", "USER", "B J Hunt", "SHP",  "One Johnny Blvd, Rogers, AR, USA ", "9058063535", "jess@jbhunt.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","1200", "USER", "R&S Trading", "AH",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "larry@rr.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","100", "USER", "Ashley Hart", "TRD",  "Morrisville Parkway, #216, Morrisville, NC 27560", "9198063535", "ashley@itpeople.com", "SUNTRUST", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","200", "USER", "Sotheby", "AH",  "One Picadally Circus , #216, London, UK ", "9198063535", "admin@sotheby.com", "Standard Chartered", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","300", "USER", "Barry Smith", "TRD",  "155 Regency Parkway, #111, Cary, 27518 ", "9198063535", "barry@us.ibm.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","400", "USER", "Cindy Patterson", "TRD",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "cpatterson@hotmail.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","500", "USER", "Tamara Haskins", "TRD",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "tamara@yahoo.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","600", "USER", "NY Life", "INS",  "155 Broadway, New York, NY, USA ", "9058063535", "barry@nyl.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","700", "USER", "J B Hunt", "SHP",  "One Johnny Blvd, Rogers, AR, USA ", "9058063535", "jess@jbhunt.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","800", "USER", "R&R Trading", "AH",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "larry@rr.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","900", "USER", "Gregory Huffman", "TRD",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "tamara@yahoo.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","1000", "USER", "Texas Life", "INS",  "155 Broadway, New York, NY, USA ", "9058063535", "barry@nyl.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","1100", "USER", "B J Hunt", "SHP",  "One Johnny Blvd, Rogers, AR, USA ", "9058063535", "jess@jbhunt.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostUser","1200", "USER", "R&S Trading", "AH",  "155 Sunset Blvd, Beverly Hills, CA, USA ", "9058063535", "larry@rr.com", "RBC Centura", "0001732345", "0234678", "2017-01-02 15:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi

	verifyResult $? "POST User request execution on of the PEERs failed "
	wait $SLEEP_TIME

	echo "==== postUsers() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function getUsers() {
	local CH_NUM=$1
	local CHAIN_NUM=$2
	local PEER=$3

	setGlobals $3

	for ((id = $START_COUNT; id <= $END_COUNT; id = $id + $INTERVAL)); do
		peer chaincode query -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\": [\"qGetUser\", \"$id\"]}"
		verifyResult $? "getUsers() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	done

	echo "==== getUsers() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function downloadImages() {
	local CH_NUM=$1
	local CHAIN_NUM=$2
	local PEER=$3

	setGlobals $3

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iDownloadImages", "DOWNLOAD"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "downloadImages() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iDownloadImages", "DOWNLOAD"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "downloadImages() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi

	echo "==== downloadImages() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function postItems() {
	local CH_NUM=$1
	local CHAIN_NUM=$2

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "100", "ARTINV", "Shadows by Asppen", "Asppen Messer", "20140202", "Original", "landscape", "Canvas", "15 x 15 in", "art1.png","600", "100", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "200", "ARTINV", "modern Wall Painting", "Scott Palmer", "20140202", "Reprint", "landscape", "Acrylic", "10 x 10 in", "art2.png","2600", "200", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "300", "ARTINV", "Splash of Color", "Jennifer Drew", "20160115", "Reprint", "modern", "Water Color", "15 x 15 in", "art3.png","1600", "300", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "400", "ARTINV", "Female Water Color", "David Crest", "19900115", "Original", "modern", "Water Color", "12 x 17 in", "art4.png","9600", "400", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "500", "ARTINV", "Nature", "James Thomas", "19900115", "Original", "modern", "Water Color", "12 x 17 in", "item-001.jpg","1800", "500", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "600", "ARTINV", "Ladys Hair", "James Thomas", "19900115", "Original", "landscape", "Acrylic", "12 x 17 in", "item-002.jpg","1200", "600", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "700", "ARTINV", "Flowers", "James Thomas", "19900115", "Original", "modern", "Acrylic", "12 x 17 in", "item-003.jpg","1000", "700", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "800", "ARTINV", "Women at work", "James Thomas", "19900115", "Original", "modern", "Acrylic", "12 x 17 in", "item-004.jpg","1500", "800", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "900", "ARTINV", "People", "James Thomas", "19900115", "Original", "modern", "Acrylic", "12 x 17 in", "people.gif","900", "900", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on PEER1.ORG2 on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "1000", "ARTINV", "Shadows by Asppen", "Asppen Messer", "20140202", "Original", "landscape", "Canvas", "15 x 15 in", "art5.png","600", "1000", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on PEER1.ORG2 on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "100", "ARTINV", "Shadows by Asppen", "Asppen Messer", "20140202", "Original", "landscape", "Canvas", "15 x 15 in", "art1.png","600", "100", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "200", "ARTINV", "modern Wall Painting", "Scott Palmer", "20140202", "Reprint", "landscape", "Acrylic", "10 x 10 in", "art2.png","2600", "200", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "300", "ARTINV", "Splash of Color", "Jennifer Drew", "20160115", "Reprint", "modern", "Water Color", "15 x 15 in", "art3.png","1600", "300", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "400", "ARTINV", "Female Water Color", "David Crest", "19900115", "Original", "modern", "Water Color", "12 x 17 in", "art4.png","9600", "400", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "500", "ARTINV", "Nature", "James Thomas", "19900115", "Original", "modern", "Water Color", "12 x 17 in", "item-001.jpg","1800", "500", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "600", "ARTINV", "Ladys Hair", "James Thomas", "19900115", "Original", "landscape", "Acrylic", "12 x 17 in", "item-002.jpg","1200", "600", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "700", "ARTINV", "Flowers", "James Thomas", "19900115", "Original", "modern", "Acrylic", "12 x 17 in", "item-003.jpg","1000", "700", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "800", "ARTINV", "Women at work", "James Thomas", "19900115", "Original", "modern", "Acrylic", "12 x 17 in", "item-004.jpg","1500", "800", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

		setGlobals $(((RANDOM % 3)))
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "900", "ARTINV", "People", "James Thomas", "19900115", "Original", "modern", "Acrylic", "12 x 17 in", "people.gif","900", "900", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args":["iPostItem", "1000", "ARTINV", "Shadows by Asppen", "Asppen Messer", "20140202", "Original", "landscape", "Canvas", "15 x 15 in", "art5.png","600", "1000", "2017-01-23 14:04:05"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi

	wait $SLEEP_TIME
	echo "==== postItems() transaction on $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function postAuction() {
	local CH_NUM=$1
	local CHAIN_NUM=$2
	setGlobals $3

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\":[\"iPostAuctionRequest\", \"$4\", \"AUCREQ\", \"$5\", \"200\", \"$6\", \"04012016\", \"$7\", \"$8\", \"INIT\", \"2017-02-13 09:05:00\", \"2017-02-13 09:05:00\", \"2017-02-13 09:10:00\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postAuction() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		echo "{\"Args\":[\"iPostAuctionRequest\", \"$4\", \"AUCREQ\", \"$5\", \"200\", \"$6\", \"04012016\", \"$7\", \"$8\", \"INIT\", \"2017-02-13 09:05:00\", \"2017-02-13 09:05:00\", \"2017-02-13 09:10:00\"]}"
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\":[\"iPostAuctionRequest\", \"$4\", \"AUCREQ\", \"$5\", \"200\", \"$6\", \"04012016\", \"$7\", \"$8\", \"INIT\", \"2017-02-13 09:05:00\", \"2017-02-13 09:05:00\", \"2017-02-13 09:10:00\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "postAuction() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi

	wait $SLEEP_TIME
	echo "==== postAuction($4,$5,$6,$7,$8) transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function openAuctionRequestForBids() {
	local CH_NUM=$1
	local CHAIN_NUM=$2
	setGlobals $3

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\":[\"iOpenAuctionForBids\", \"$4\", \"OPENAUC\", \"$5\", \"2017-02-13 09:18:00\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "openAuctionRequestForBids() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\":[\"iOpenAuctionForBids\", \"$4\", \"OPENAUC\", \"$5\", \"2017-02-13 09:18:00\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "openAuctionRequestForBids() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi
	wait $SLEEP_TIME

	echo "==== openAuctionRequestForBids($4,$5) transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function submitBids() {
	local CH_NUM=$1
	local CHAIN_NUM=$2
	setGlobals $3

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\":[\"iPostBid\", \"$4\", \"BID\", \"$5\", \"$6\", \"$7\", \"$8\", \"2017-02-13 09:19:01\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "submitBids() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\":[\"iPostBid\", \"$4\", \"BID\", \"$5\", \"$6\", \"$7\", \"$8\", \"2017-02-13 09:19:01\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "submitBids() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi
	wait $SLEEP_TIME

	echo "==== submitBids($4,$5,$6,$7,$8) transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function closeAuction() {
	local CH_NUM=$1
	local CHAIN_NUM=$2
	setGlobals $3

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args": ["iCloseOpenAuctions", "2016", "CLAUC", "2017-01-23 13:53:00.3 +0000 UTC"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "closeAuction() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c '{"Args": ["iCloseOpenAuctions", "2016", "CLAUC", "2017-01-23 13:53:00.3 +0000 UTC"]}' --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "closeAuction() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi

	wait $SLEEP_TIME

	setGlobals $3
	peer chaincode query -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\": [\"qGetItem\", \"$4\"]}" &>$TEMP_LOG_FILE
	verifyResult $? "getItem() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	cat $TEMP_LOG_FILE >>$LOG_FILE

	OUTPUT=$(cat $TEMP_LOG_FILE | awk -F 'Result: | 2017' '{print $2}')
	USER_ID=$(echo $OUTPUT | jq ".CurrentOwnerID")
	AES_KEY=$(echo $OUTPUT | jq ".AES_Key")
	echo $OUTPUT | awk -F ': |\n' '{print $2}' | jq "."
	wait $SLEEP_TIME

	echo "==== closeAuction() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

function transferItem() {
	local CH_NUM=$1
	local CHAIN_NUM=$2
	setGlobals $3

	if [ -z "$CORE_PEER_TLS_ENABLED" -o "$CORE_PEER_TLS_ENABLED" = "false" ]; then
		peer chaincode invoke -o $ORDERER_IP -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\": [\"iTransferItem\", \"$4\", $USER_ID, $AES_KEY, \"$4\", \"XFER\",\"2017-01-24 11:00:00\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "transferItem() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	else
		peer chaincode invoke -o $ORDERER_IP --tls $CORE_PEER_TLS_ENABLED --cafile $ORDERER_CA -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\": [\"iTransferItem\", \"$4\", $USER_ID, $AES_KEY, \"$4\", \"XFER\",\"2017-01-24 11:00:00\"]}" --logging-level=$LOG_LEVEL >>$LOG_FILE
		verifyResult $? "transferItem() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"
	fi

	wait $SLEEP_TIME

	peer chaincode query -C $CHANNEL_NAME$CH_NUM -n $CHAINCODE_NAME$CHAIN_NUM -c "{\"Args\": [\"qGetItem\", \"$4\"]}" >>$LOG_FILE
	verifyResult $? "getItem() transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM failed"

	wait $SLEEP_TIME

	echo "==== transferItem($4) transaction on PEER$3 $CHANNEL_NAME$CH_NUM/$CHAINCODE_NAME$CHAIN_NUM is successful ==== " >>$LOG_FILE
}

echo "==== Args Passed: CHANNEL_NAME:${CHANNEL_NAME}, CHANNELS:${CHANNELS}, CHAINCODES:${CHAINCODES}, ENDORSERS:${ENDORSERS}, RUN_TIME_HOURS:${RUN_TIME_HOURS}, CHANNEL_NAME:${FN_NAME}, FN_NAME:${FN_NAME} for Auction API Driver ==== " >>$LOG_FILE

if [ "${FN_NAME}" = "createChannel" ]; then
	for ((ch = 0; $ch < $CHANNELS; ch++)); do
		createChannel $ch
	done
	echo "==== Creating Channel is successful ==== "

elif [ "${FN_NAME}" = "joinChannel" ]; then
	joinChannel
	## Set the anchor peers for each org in the channel
	updateAnchorPeers 0 0
	updateAnchorPeers 2 0
	echo "==== Join Channel is successful ==== "

elif [ "${FN_NAME}" = "installChaincode" ]; then
	## Install chaincode on all peers
	installChaincode
	echo "==== Installing chaincode is successful ==== "

elif [ "${FN_NAME}" = "instantiateChaincode" ]; then
	instantiateChaincode 0
	echo "==== Instantiating chaincode is successful ==== "

elif [ "${FN_NAME}" = "postUsers" ]; then
	## POST USERS on all peers
	for ((ch = 0; $ch < $CHANNELS; ch++)); do
		for ((chain = 0; $chain < $CHAINCODES; chain++)); do
			postUsers $ch $chain
		done
	done
	echo "==== Posting Users transaction is successful ==== "

elif [ "${FN_NAME}" = "getUsers" ]; then
	for ((ch = 0; $ch < $CHANNELS; ch++)); do
		for ((chain = 0; $chain < $CHAINCODES; chain++)); do
			for ((peer_number = 0; peer_number < $ENDORSERS; peer_number++)); do
				getUsers $ch $chain $peer_number
			done
		done
	done
	echo "==== Get Users transaction is successful ==== "

elif [ "${FN_NAME}" = "downloadImages" ]; then
	for ((ch = 0; $ch < $CHANNELS; ch++)); do
		for ((chain = 0; $chain < $CHAINCODES; chain++)); do
			for ((peer_number = 0; peer_number < $ENDORSERS; peer_number++)); do
				downloadImages $ch $chain $peer_number
			done
		done
	done
	echo "==== Download Images transaction is successful ==== "

elif [ "${FN_NAME}" = "postItems" ]; then
	for ((ch = 0; $ch < $CHANNELS; ch++)); do
		for ((chain = 0; $chain < $CHAINCODES; chain++)); do
			postItems $ch $chain
		done
	done
	echo "==== Post Items transaction is successful ==== "

elif [ "${FN_NAME}" = "auctionInvokes" ]; then
	auctionindex=1000
	bidPrice=5000
	bidNumber=1000

	START=$(date +%s)

	while [ $(($(date +%s) - (60 * 60 * $RUN_TIME_HOURS))) -lt $START ]; do
		for ((ch = 0; $ch < $CHANNELS; ch++)); do
			for ((chain = 0; $chain < $CHAINCODES; chain++)); do
				for ((userid = $START_COUNT; userid <= $END_COUNT; userid = $userid + $INTERVAL)); do
					postAuction $ch $chain $(((RANDOM % 3))) $auctionindex $userid $userid $auctionindex $auctionindex
					openAuctionRequestForBids $ch $chain $(((RANDOM % 3))) $auctionindex 10
					for ((biduserid = $START_COUNT; biduserid <= $END_COUNT; biduserid = $biduserid + $INTERVAL)); do
						if [ $biduserid -ne $userid ]; then
							submitBids $ch $chain $(((RANDOM % 3))) $auctionindex $bidNumber $userid $biduserid $bidPrice
							bidNumber=$(expr $bidNumber + 1)
							bidPrice=$(expr $bidPrice + 1)
						fi
					done
					closeAuction $ch $chain $(((RANDOM % 3))) $userid
					transferItem $ch $chain $(((RANDOM % 3))) $userid
					auctionindex=$(expr $auctionindex + 1)
				done
			done
		done
	done
	END=$(date +%s)

	runtime=$((END - START))

	echo "==== Auction Invokes Run Time :  $runtime ==== " >>$LOG_FILE
	echo "==== Open Auction/Submit Bids/Close Auction/Transfer Item transaction(s) are successful ==== "
else
	echo "==== Invalid Function Name ${FN_NAME} for Auction API Driver ==== " >>$LOG_FILE
	exit 1
fi
