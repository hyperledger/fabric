#!/bin/bash

set -eux

# Compile protos
ALL_PROTO_FILES="$(find . -name "*.proto" -exec readlink -f {} \;)"
PROTO_DIRS="$(dirname $ALL_PROTO_FILES | sort | uniq)"
PROTO_DIRS_WITHOUT_JAVA_AND_SDK="$(printf '%s\n' $PROTO_DIRS | grep -v "shim/java" | grep -v "/sdk")"
for dir in $PROTO_DIRS_WITHOUT_JAVA_AND_SDK; do
        cd "$dir"
	protoc --proto_path="$dir" --go_out=plugins=grpc:. "$dir"/*.proto
done
