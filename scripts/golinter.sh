#!/bin/bash
#
# Copyright Greg Haskins All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e

declare -a arr=(
"./bccsp"
"./common"
"./core"
"./events"
"./examples"
"./gossip"
"./msp"
"./orderer"
"./peer"
"./protos"
)

for i in "${arr[@]}"
do
    echo "Checking $i"
    go vet $i/...
    OUTPUT="$(goimports -srcdir $GOPATH/src/github.com/hyperledger/fabric -l $i)"
    if [[ $OUTPUT ]]; then
	echo "The following files contain goimports errors"
	echo $OUTPUT
	echo "The goimports command must be run for these files"
	exit 1
    fi
done
