#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

if [[ ! $(git diff-tree --no-commit-id --name-only 'HEAD^..HEAD') == "docs" ]]; then
  echo "::set-output name=run-tests::true"
fi
