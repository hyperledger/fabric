#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

CHECK=$(git diff --name-only HEAD * | grep -v .png$ | grep -v .git | grep -v ^CHANGELOG \
  | grep -v ^vendor/ | grep -v ^build/ | sort -u)

if [[ -z "$CHECK" ]]; then
  CHECK=$(git diff-tree --no-commit-id --name-only -r $(git log -2 \
    --pretty=format:"%h") | grep -v .png$ | grep -v .git | grep -v ^CHANGELOG \
    | grep -v ^vendor/ | grep -v ^build/ | sort -u)
fi

echo "Checking changed go files for spelling errors ..."
errs=`echo $CHECK | xargs misspell -source=text`
if [ -z "$errs" ]; then
   echo "spell checker passed"
   exit 0
fi
echo "The following files are have spelling errors:"
echo "$errs"
exit 0
