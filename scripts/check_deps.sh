#!/bin/bash

# Copyright IBM Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# create temporary directory for go.mod and clean it
# up when the script exists.
dep_tempdir="$(mktemp -d "$(basename "$0")"-XXXXX)"
trap 'rm -rf "$dep_tempdir"' EXIT

# copy go.mod and go.sum to the temporary directory we created
fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"
cp "${fabric_dir}/go.mod" "${dep_tempdir}/"
cp "${fabric_dir}/go.sum" "${dep_tempdir}/"

# check if we have unused requirements
go mod tidy -modfile="${dep_tempdir}/go.mod"

for f in go.mod go.sum; do
    if ! diff -q "${fabric_dir}/$f" "${dep_tempdir}/$f"; then
        echo "It appears $f is stale. Please run 'go mod tidy' and 'go mod vendor'."
        diff -u "${fabric_dir}/$f" "${dep_tempdir}/$f"
        exit 1
    fi
done
