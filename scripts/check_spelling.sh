#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


echo "Checking Go files for spelling errors ..."
errs=`find . -name "*.go" | grep -v vendor/ | grep -v build/ | grep -v ".pb.go" | xargs misspell`
if [ -z "$errs" ]; then
   echo "spell checker passed"
   exit 0
fi
echo "The following files are have spelling errors:"
echo "$errs"
exit 1
