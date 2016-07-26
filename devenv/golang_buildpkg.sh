#!/bin/sh
if [ -z "$1" ]; then
        echo 'GOOS is not specified' 1>&2
        exit 2
else
        export GOOS=$1
        if [ "$GOOS" = "windows" ]; then
                export CGO_ENABLED=0
        fi
fi
shift
if [ -n "$1" ]; then
        export GOARCH=$1
fi
cd $GOROOT/src
go tool dist install -v pkg/runtime
go install -v -a std
