#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


UP_DOWN=$1
CH_NAME=$2
CHANNELS=$3
CHAINCODES=$4
ENDORSERS=$5

: ${CH_NAME:="mychannel"}
: ${CHANNELS:="1"}
: ${CHAINCODES:="1"}
: ${ENDORSERS:="4"}
COMPOSE_FILE=docker-compose.yaml

function printHelp () {
	echo "Usage: ./network_setup.sh <up|down|retstart> [channel-name] [total-channels [chaincodes] [endorsers count]"
}

function validateArgs () {
	if [ -z "${UP_DOWN}" ]; then
		echo "Option up / down / restart not mentioned"
		printHelp
		exit 1
	fi
}

function clearContainers () {
        CONTAINER_IDS=$(docker ps -aq)
        if [ -z "$CONTAINER_IDS" -o "$CONTAINER_IDS" = " " ]; then
                echo "---- No containers available for deletion ----"
        else
                docker rm -f $CONTAINER_IDS
        fi
}

function removeUnwantedImages() {
        DOCKER_IMAGE_IDS=$(docker images | grep "dev\|none\|test-vp\|peer[0-9]-" | awk '{print $3}')
        if [ -z "$DOCKER_IMAGE_IDS" -o "$DOCKER_IMAGE_IDS" = " " ]; then
                echo "---- No images available for deletion ----"
        else
                docker rmi -f $DOCKER_IMAGE_IDS
        fi
}

function networkUp () {
        CurrentDIR=$PWD
        echo "ch_name $CH_NAME"
        echo "Num channels $CHANNELS"
        echo "Num chaincodes $CHAINCODES"
        echo "Num of endorsers/peers $ENDORSERS"
        source generateCfgTrx.sh $CH_NAME $CHANNELS
        cd $CurrentDIR

	docker-compose -f $COMPOSE_FILE up -d 2>&1
	if [ $? -ne 0 ]; then
		echo "ERROR !!!! Unable to pull the images "
		exit 1
	fi
}

function networkDown () {
        docker-compose -f $COMPOSE_FILE down
        #Cleanup the chaincode containers
	clearContainers
	#Cleanup images
	removeUnwantedImages
        #remove orderer and config txn
        rm -rf channel-artifacts/*  crypto-config  crypto-config.yaml configtx.yaml __pycache__ results.xml scripts/.cache base
}

validateArgs

#Create the network using docker compose
if [ "${UP_DOWN}" == "up" ]; then
	networkUp
elif [ "${UP_DOWN}" == "down" ]; then ## Clear the network
	networkDown
elif [ "${UP_DOWN}" == "restart" ]; then ## Restart the network
	networkDown
	networkUp
else
	printHelp
	exit 1
fi
