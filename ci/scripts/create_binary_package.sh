#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

make "release/${TARGET}"
mkdir "release/${TARGET}/config"
mv sampleconfig/*yaml "release/${TARGET}/config"
cd "release/${TARGET}"
if [ "$TARGET" == "windows-amd64" ]; then
    for FILE in bin/*; do mv $FILE $FILE.exe; done
fi
tar -czvf "hyperledger-fabric-${TARGET}-${RELEASE}.tar.gz" bin config
