#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -x -euo pipefail

git log -5 |cat

git status

if [[ $(git diff-tree --no-commit-id --name-only 'HEAD^..HEAD') == "docs" ]]; then
  echo "##vso[task.setvariable variable=buildDoc;isOutput=true]true"
else
  echo "##vso[task.setvariable variable=buildDoc;isOutput=true]true"
  echo "##vso[task.setvariable variable=runTests;isOutput=true]true"
fi