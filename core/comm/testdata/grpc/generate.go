/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// +build ignore

//go:generate protoc --proto_path=$GOPATH/src/github.com/hyperledger/fabric/core/comm/testdata/grpc --go_out=plugins=grpc:$GOPATH/src test.proto

package grpc
