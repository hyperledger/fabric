#!/bin/bash

#
# Copyright IBM Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# Checks whether any files using go:generate directives make
# references to their parent directories, instead of generating
# the mock into the directory where it is being utilized
echo "Checking for go:generate parent path references"
OUTPUT="$(git ls-files "*.go" | grep -Ev 'vendor' | xargs grep 'go:generate.*\.\.')"
if [[ -n "$OUTPUT" ]]; then
    echo "The following files contain references to parent directories in their go:generate directives."
    echo "Creating mocks in directories in which they are not used, can create errors that are not"
    echo "easily discoverable for code refactors that move the referenced code. It also implies that"
    echo "a mock for a remote interface is being defined in some place other that where it is being used"
    echo "It is best practice to generate the mock in the directory in which it will be used."
    echo "$OUTPUT"
    exit 1
fi
