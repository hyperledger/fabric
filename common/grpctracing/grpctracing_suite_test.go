/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpctracing_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/grpctracing/testpb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate protoc --proto_path=testpb --go_out=plugins=grpc,paths=source_relative:testpb testpb/echo.proto

func TestGrpctracing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grpctracing Suite")
}

//go:generate counterfeiter -o fakes/echo_service.go --fake-name EchoServiceServer . echoServiceServer

type echoServiceServer interface {
	testpb.EchoServiceServer
}
