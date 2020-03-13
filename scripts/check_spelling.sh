#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

filter() {
    while read -r data; do
        grep -Ev '^CHANGELOG|\.git|\.png$|^vendor/' <<< "$data"
    done
}

CHECK=$(git diff --name-only HEAD -- * | filter)

if [[ -z "$CHECK" ]]; then
    CHECK=$(git diff-tree --no-commit-id --name-only -r HEAD^..HEAD | filter)
fi

echo "Checking changed go files for spelling errors ..."
errs=$(echo "$CHECK" | xargs misspell -source=text)
if [ -z "$errs" ]; then
    echo "spell checker passed"
    exit 0
fi
echo "The following files are have spelling errors:"
echo "$errs"
exit 0
