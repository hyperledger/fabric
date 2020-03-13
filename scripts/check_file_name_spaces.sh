#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -e

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"

echo "Checking files for spaces in file names..."

filenames=$(find "$fabric_dir" -name '* *')
if [ -n "$filenames" ]; then
    echo "The following file names contain spaces:"
    echo "$filenames"
    echo "File names must not contain spaces."
    exit 1
fi