#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


if [ "$2" != "hyperledger" ]; then

        echo " Pull Request number is $1 "
        echo " User Name is $2 "
	echo " Repository Name is $3 "

mkdir -p $HOME/gopath/src/github.com/hyperledger

	echo "hyperledger/fabric folder created"

git clone -ql $HOME/gopath/src/github.com/$2/$3 $HOME/gopath/src/github.com/hyperledger/fabric

	echo "linked $2 user repo into hyperledger/fabric folder"

fi
