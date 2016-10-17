#!/bin/bash

target=$1

deps=`go list -f '{{ join .Deps "\n" }}' $target`

find $GOPATH/src/$target -type f

for dep in $deps;
do
    importpath=$GOPATH/src/$dep
    if [ -d $importpath ];
    then
        find $importpath -type f
    fi
done
