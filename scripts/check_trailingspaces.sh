#!/bin/bash
# Copyright Hitachi, Ltd. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

COMMIT_FILES=$(git diff --name-only --diff-filter=ACMRTUXB HEAD | grep -Ev '(^|/)vendor/')

echo "Checking trailing spaces ..."
for filename in $COMMIT_FILES; do
    if [[ $(file "$filename") == *"ASCII text"* ]]; then
        if grep -El " +$" "$filename"; then
            FOUND_TRAILING='yes'
            echo "Error: Trailing spaces found in file:$filename, lines:"
            grep -En " +$" "$filename"
        fi
    fi
done

if [[ -n ${FOUND_TRAILING+x} ]]; then
    echo "Please omit trailing spaces and make again."
    exit 1
fi
