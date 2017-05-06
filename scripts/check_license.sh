#!/bin/bash

echo "Checking Go files for license headers ..."
missing=`grep -l -L "Apache License" \`find . -name "*.go"\` | grep -v "./vendor" | grep -v "build/docker/gotools" | grep -v ".pb.go" | grep -v "examples/chaincode/go/utxo/consensus/consensus.go"`
if [ $? -eq 0 ]; then
   echo "The following files are missing license headers:"
   echo "$missing"
   exit 1
fi
echo "All go files have license headers"
