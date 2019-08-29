#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -eux -o pipefail

# Find all proto dirs to be processed
PROTO_DIRS=$(eval \
    find "$(pwd) \
    -path $(pwd)/vendor -prune -o \
    -path $(pwd)/.build -prune -o \
    -name '*.proto' \
    -exec readlink -f {} \; \
    -print0" | xargs -n 1 dirname | sort -u)

for dir in ${PROTO_DIRS}; do
    echo Working on dir "$dir"
    # As this is a proto root, and there may be subdirectories with protos, compile the protos for each sub-directory which contains them
    for protos in $(find "$dir" -name '*.proto' -exec dirname {} \; | sort | uniq) ; do
        protoc --proto_path="$dir" --go_out=plugins=grpc:"$GOPATH"/src "$protos"/*.proto
    done
done
