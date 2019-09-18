#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -eux -o pipefail

# Find all proto dirs to be processed
PROTO_DIRS="$(find "$(pwd)" \
    -path "$(pwd)/vendor" -prune -o \
    -path "$(pwd)/build" -prune -o \
    -name '*.proto' -print0 | \
    xargs -0 -n 1 dirname | \
    sort -u | grep -v testdata)"

for dir in ${PROTO_DIRS}; do
    protoc --proto_path="$dir" --go_out=plugins=grpc,paths=source_relative:"$dir" "$dir"/*.proto
done
