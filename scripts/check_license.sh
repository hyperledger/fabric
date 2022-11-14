#!/bin/bash
#
# Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -euo pipefail

# shellcheck source=/dev/null
source "$(cd "$(dirname "$0")" && pwd)/functions.sh"

CHECK=$(git diff --name-only --diff-filter=ACMRTUXB HEAD | tr '\n' ' ')
if [[ -z "$CHECK" ]]; then
    CHECK=$(git diff-tree --no-commit-id --name-only --diff-filter=ACMRTUXB -r "HEAD^..HEAD" | tr '\n' ' ')
fi

FILTERED=$(filterExcludedAndGeneratedFiles "$CHECK")
if [[ -z "$FILTERED" ]]; then
    echo "All files are excluded from having license headers"
    exit 0
fi

missing=$(echo "$FILTERED" | sort -u |  xargs ls -d 2>/dev/null | xargs grep -L "SPDX-License-Identifier") || true
if [[ -z "$missing" ]]; then
    echo "All files have SPDX-License-Identifier headers"
    exit 0
fi
echo "The following files are missing SPDX-License-Identifier headers:"
echo "$missing"
echo
echo "Please replace the Apache license header comment text with:"
echo "SPDX-License-Identifier: Apache-2.0"

echo
echo "Checking committed files for traditional Apache License headers ..."
missing=$(echo "$missing" | xargs ls -d 2>/dev/null | xargs grep -L "http://www.apache.org/licenses/LICENSE-2.0") || true
if [[ -z "$missing" ]]; then
    echo "All remaining files have Apache 2.0 headers"
    exit 0
fi
echo "The following files are missing traditional Apache 2.0 headers:"
echo "$missing"
echo "Fatal Error - All files must have a license header"
exit 1
