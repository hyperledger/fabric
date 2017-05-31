#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


echo "Checking Go files for license headers ..."
missing=`find . -name "*.go" | grep -v build/ | grep -v vendor/ | grep -v ".pb.go" | grep -v "examples/chaincode/go/utxo/consensus/consensus.go" | xargs grep -l -L "Apache License"`
if [ -z "$missing" ]; then
   echo "All go files have license headers"
   exit 0
fi
echo "The following files are missing license headers:"
echo "$missing"
exit 1
