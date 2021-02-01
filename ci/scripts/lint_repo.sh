#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

npm install repolinter
node ./node_modules/repolinter/bin/repolinter.js lint .
