#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -eux

# To set a proto root for a set of protos, create a .protoroot file in one of the parent directories
# which you wish to use as the proto root.  If no .protoroot file exists within fabric/.../<your_proto>
# then the proto root for that proto is inferred to be its containing directory.

# Find explicit proto roots
PROTO_ROOT_FILES="$(find . -name ".protoroot" -exec readlink -f {} \;)"
PROTO_ROOT_DIRS="$(dirname $PROTO_ROOT_FILES)"


# Find all proto files to be compiled, excluding any which are in a proto root or in the vendor folder
ROOTLESS_PROTO_FILES="$(find $PWD \
                             $(for dir in $PROTO_ROOT_DIRS ; do echo "-path $dir -prune -o " ; done) \
                             -path $PWD/vendor -prune -o \
                             -path $PWD/.build -prune -o \
                             -path $PWD/core/chaincode/shim/java -prune -o \
                             -name "*.proto" -exec readlink -f {} \;)"
ROOTLESS_PROTO_DIRS="$(dirname $ROOTLESS_PROTO_FILES | sort | uniq)"

for dir in $ROOTLESS_PROTO_DIRS $PROTO_ROOT_DIRS; do
echo Working on dir $dir
        # As this is a proto root, and there may be subdirectories with protos, compile the protos for each sub-directory which contains them
	for protos in $(find "$dir" -name '*.proto' -exec dirname {} \; | sort | uniq) ; do
	       protoc --proto_path="$dir" --go_out=plugins=grpc:$GOPATH/src "$protos"/*.proto
	done
done
