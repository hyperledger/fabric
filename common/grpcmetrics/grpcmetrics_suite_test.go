/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpcmetrics_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/grpcmetrics/testpb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate protoc --proto_path=$GOPATH/src/github.com/hyperledger/fabric/common/grpcmetrics/testpb --go_out=plugins=grpc:$GOPATH/src $GOPATH/src/github.com/hyperledger/fabric/common/grpcmetrics/testpb/echo.proto

func TestGrpcmetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grpcmetrics Suite")
}

//go:generate counterfeiter -o fakes/echo_service.go --fake-name EchoServiceServer . echoServiceServer

type echoServiceServer interface {
	testpb.EchoServiceServer
}
