#!/bin/bash -e

# Copyright Greg Haskins All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

declare -a arr=(
"./bccsp"
"./common"
"./core"
"./discovery"
"./events"
"./examples"
"./gossip"
"./idemix"
"./msp"
"./orderer"
"./peer"
"./protos"
)

# place the Go build cache directory into the default build tree if it exists
if [ -d "${GOPATH}/src/github.com/hyperledger/fabric/.build" ]; then
    export GOCACHE="${GOPATH}/src/github.com/hyperledger/fabric/.build/go-cache"
fi

for i in "${arr[@]}"
do
    echo ">>>Checking code under $i/"

    echo "Checking with gofmt"
    OUTPUT="$(gofmt -l -s ./$i | grep -v testdata/ || true)"
    if [[ $OUTPUT ]]; then
        echo "The following files contain gofmt errors"
        echo "$OUTPUT"
        echo "The gofmt command 'gofmt -l -s -w' must be run for these files"
        exit 1
    fi

    echo "Checking with go vet"
    OUTPUT="$(go vet -composites=false $i/...)"
    if [[ $OUTPUT ]]; then
        echo "The following files contain go vet errors"
        echo $OUTPUT
        exit 1
    fi
done
