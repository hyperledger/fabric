#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

# Check TARGET is non-empty
if [[ -z "$TARGET" ]]; then
  echo "Error: TARGET is not set."
  exit 1
fi
make "release/${TARGET}"
mkdir -p "release/${TARGET}/config"

# cp not move otherwise this breaks your source tree
cp sampleconfig/*yaml "release/${TARGET}/config"

cd "release/${TARGET}"
if [ "$TARGET" == "windows-amd64" ]; then
    for FILE in bin/*; do mv $FILE $FILE.exe; done
fi

# Trim the semrev 'v' from the start of the RELEASE attribute
VERSION=$(echo $RELEASE | sed -e  's/^v\(.*\)/\1/')

tar -czvf "hyperledger-fabric-${TARGET}-${VERSION}.tar.gz" bin config builders
