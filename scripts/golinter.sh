#!/bin/bash

# Copyright Greg Haskins All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

set -e

# shellcheck source=/dev/null
source "$(cd "$(dirname "$0")" && pwd)/functions.sh"

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"
source_dirs=()
while IFS=$'\n' read -r source_dir; do
    source_dirs+=("$source_dir")
done < <(go list -f '{{.Dir}}' ./... | sed s,"${fabric_dir}".,,g | cut -f 1 -d / | sort -u)

echo "Checking with gofumpt"
OUTPUT="$(gofumpt -l "${source_dirs[@]}")"
OUTPUT="$(filterExcludedAndGeneratedFiles "$OUTPUT")"
if [ -n "$OUTPUT" ]; then
    echo "The following files contain gofumpt errors"
    echo "$OUTPUT"
    echo "The gofumpt command 'gofumpt -l -w' must be run for these files"
    exit 1
fi

# staticcheck Fabric source files - ignore issues in vendored dependency projects
echo "Checking with staticcheck"
OUTPUT="$(staticcheck ./... | grep -v vendor/ || true)"
if [ -n "$OUTPUT" ]; then
    echo "The following staticcheck issues were flagged"
    echo "$OUTPUT"
    exit 1
fi

echo "Checking with golangci-lint"
if ! OUTPUT="$(golangci-lint run ./... 2>/dev/null)"; then
    echo "The following golangci-lint issues were flagged"
    echo "$OUTPUT"
    exit 1
fi
