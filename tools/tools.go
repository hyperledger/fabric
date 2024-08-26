//go:build tools
// +build tools

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tools

import (
	_ "github.com/AlekSi/gocov-xml"
	_ "github.com/axw/gocov/gocov"
	_ "github.com/client9/misspell/cmd/misspell"
	_ "github.com/go-swagger/go-swagger/cmd/swagger"
	_ "github.com/hyperledger/fabric-lib-go/common/metrics/cmd/gendoc"
	_ "github.com/maxbrunsfeld/counterfeiter/v6"
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "github.com/vektra/mockery/v2"
	_ "golang.org/x/lint/golint"
	_ "golang.org/x/tools/cmd/goimports"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	_ "honnef.co/go/tools/cmd/staticcheck"
	_ "mvdan.cc/gofumpt"
)
