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

echo "Checking with goimports"
OUTPUT="$(goimports -l "${source_dirs[@]}")"
OUTPUT="$(filterExcludedAndGeneratedFiles "$OUTPUT")"
if [ -n "$OUTPUT" ]; then
    echo "The following files contain goimports errors"
    echo "$OUTPUT"
    echo "The goimports command 'goimports -l -w' must be run for these files"
    exit 1
fi

echo "Checking with gofumpt"
OUTPUT="$(gofumpt -l -s "${source_dirs[@]}")"
OUTPUT="$(filterExcludedAndGeneratedFiles "$OUTPUT")"
if [ -n "$OUTPUT" ]; then
    echo "The following files contain gofumpt errors"
    echo "$OUTPUT"
    echo "The gofumpt command 'gofumpt -l -s -w' must be run for these files"
    exit 1
fi

# Now that context is part of the standard library, we should use it
# consistently. The only place where the legacy golang.org version should be
# referenced is in the generated protos.
echo "Checking for golang.org/x/net/context"
# shellcheck disable=SC2016
TEMPLATE='{{with $d := .}}{{range $d.Imports}}{{ printf "%s:%s " $d.ImportPath . }}{{end}}{{end}}'
OUTPUT="$(go list -f "$TEMPLATE" ./... | grep 'golang.org/x/net/context' | cut -f1 -d:)"
if [ -n "$OUTPUT" ]; then
    echo "The following packages import golang.org/x/net/context instead of context"
    echo "$OUTPUT"
    exit 1
fi

# We use golang/protobuf but goimports likes to add gogo/protobuf.
# Prevent accidental import of gogo/protobuf.
echo "Checking for github.com/gogo/protobuf"
# shellcheck disable=SC2016
TEMPLATE='{{with $d := .}}{{range $d.Imports}}{{ printf "%s:%s " $d.ImportPath . }}{{end}}{{end}}'
OUTPUT="$(go list -f "$TEMPLATE" ./... | grep 'github.com/gogo/protobuf' | cut -f1 -d:)"
if [ -n "$OUTPUT" ]; then
    echo "The following packages import github.com/gogo/protobuf instead of github.com/golang/protobuf"
    echo "$OUTPUT"
    exit 1
fi

echo "Checking with go vet"
PRINTFUNCS="Debug,Debugf,Print,Printf,Info,Infof,Warning,Warningf,Error,Errorf,Critical,Criticalf,Sprint,Sprintf,Log,Logf,Panic,Panicf,Fatal,Fatalf,Notice,Noticef,Wrap,Wrapf,WithMessage"
OUTPUT="$(go vet -all -printfuncs "$PRINTFUNCS" ./...)"
if [ -n "$OUTPUT" ]; then
    echo "The following files contain go vet errors"
    echo "$OUTPUT"
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
