#!/bin/bash

set -e

declare -a arr=("./consensus" "./core" "./events" "./examples" "./membersrvc" "./peer" "./protos")

for i in "${arr[@]}"
do
	OUTPUT="$(goimports -srcdir $GOPATH/src/github.com/hyperledger/fabric -l $i)"
	if [[ $OUTPUT ]]; then
		echo "The following files contain goimports errors"
		echo $OUTPUT
		echo "The goimports command must be run for these files"
		exit 1
	fi
done
