#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


export ARCH=$(echo "$(uname -s|tr '[:upper:]' '[:lower:]'|sed 's/mingw64_nt.*/windows/')-$(uname -m | sed 's/x86_64/amd64/g')" | awk '{print tolower($0)}')

curl https://nexus.hyperledger.org/content/repositories/releases/org/hyperledger/fabric/fabric-binary/${ARCH}-1.0.0-alpha2/fabric-binary-${ARCH}-1.0.0-alpha2.tar.gz | tar xz
cd release/${ARCH}
sh download-dockerimages.sh -c $(uname -m)-1.0.0-alpha2 -f $(uname -m)-1.0.0-alpha2
