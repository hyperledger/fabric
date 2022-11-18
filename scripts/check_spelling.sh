#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -euo pipefail

# Sometimes the git diff returns an empty list; grep returns 1 in this case, and
# the set -e will fail the script

# "catch exit status 1" grep wrapper
c1grep() { grep "$@" || test $? = 1; }

EXCLUDE_FILE_PATTERN="^CHANGELOG|\.git|\.png$|^vendor/"
CHECK=$(git diff --name-only HEAD -- * | c1grep -Ev $EXCLUDE_FILE_PATTERN)

if [[ -z "$CHECK" ]]; then
   CHECK=$(git diff-tree --no-commit-id --name-only -r HEAD^..HEAD | c1grep -Ev $EXCLUDE_FILE_PATTERN)
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
