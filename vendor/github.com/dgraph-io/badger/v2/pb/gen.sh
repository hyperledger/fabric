#!/bin/bash

# You might need to go get -v github.com/gogo/protobuf/...

protos=${GOPATH-$HOME/go}/src/github.com/dgraph-io/badger/pb
pushd $protos > /dev/null
protoc --gofast_out=plugins=grpc:. -I=. pb.proto

# Move pb.pb.go file to the correct directory. This is necessary because protoc
# would generate the pb.pb.go file inside a different directory.
mv $protos/github.com/dgraph-io/badger/v2/pb/pb.pb.go ./
