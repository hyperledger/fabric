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
done
