#!/bin/bash
#
# Copyright Greg Haskins All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -x
set -e

CHANNEL_NAME=$1
CHANNEL_TXNS=$2
PEERS=$3
TLS_ENABLED=$4

CA_CRT=build/cryptogen/ordererOrganizations/orderer.net/tlsca/tlsca.orderer.net-cert.pem

if [ "$TLS_ENABLED" == "true" ]; then
   CREATE_OPTS="--tls --cafile $CA_CRT"
fi

for TXN in $CHANNEL_TXNS; do
    peer channel create -o orderer:7050 \
         -c $CHANNEL_NAME \
         -f $TXN \
         $CREATE_OPTS
done

for PEER in $PEERS; do
    CORE_PEER_ADDRESS=$PEER:7051 peer channel join -b $CHANNEL_NAME.block
done
