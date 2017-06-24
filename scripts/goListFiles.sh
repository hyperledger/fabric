#!/bin/bash
#
# Copyright Greg Haskins All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


find_golang_src() {
    find $1 -name "*.go" -or -name "*.h" -or -name "*.c"
}

list_deps() {
    target=$1

    deps=`go list -f '{{ join .Deps "\n" }}' $target`

    find_golang_src $GOPATH/src/$target

    for dep in $deps;
    do
        importpath=$GOPATH/src/$dep
        if [ -d $importpath ];
        then
            find_golang_src $importpath
        fi
    done
}

list_deps $1 | sort | uniq
