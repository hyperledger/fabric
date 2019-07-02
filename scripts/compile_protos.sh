#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -eux -o pipefail

# To set a proto root for a set of protos, create a .protoroot file in one of the parent directories
# which you wish to use as the proto root.  If no .protoroot file exists within fabric/.../<your_proto>
# then the proto root for that proto is inferred to be its containing directory.
# Find explicit proto roots
PROTO_ROOT_DIRS=$(find . -name ".protoroot"  -exec readlink -f {} \; -print0 | xargs -n 1 dirname | sort -u)

ROOTLESS_PROTO_DIRS_CMD_FLAGS=();

for dir in ${PROTO_ROOT_DIRS}; do
    ROOTLESS_PROTO_DIRS_CMD_FLAGS+=("-path $dir -prune -o");
done

# Find all proto dirs to be processed, excluding any which are in a proto root or in the vendor folder

ROOTLESS_PROTO_DIRS=$(eval \
    find "$(pwd) \
    -path $(pwd)/vendor -prune -o \
    -path $(pwd)/.build -prune -o \
    ${ROOTLESS_PROTO_DIRS_CMD_FLAGS[*]} \
    -name '*.proto' \
    -exec readlink -f {} \; \
    -print0" | xargs -n 1 dirname | sort -u)

for dir in ${ROOTLESS_PROTO_DIRS} ${PROTO_ROOT_DIRS}; do
    echo Working on dir "$dir"
    # As this is a proto root, and there may be subdirectories with protos, compile the protos for each sub-directory which contains them
    for protos in $(find "$dir" -name '*.proto' -exec dirname {} \; | sort | uniq) ; do
        protoc --proto_path="$dir" --go_out=plugins=grpc:"$GOPATH"/src "$protos"/*.proto
    done
done
