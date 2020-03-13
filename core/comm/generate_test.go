/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

//go:generate protoc --proto_path=testdata/grpc --go_out=plugins=grpc,paths=source_relative:testpb testdata/grpc/test.proto

package comm_test
