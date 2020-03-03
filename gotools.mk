# Copyright the Hyperledger Fabric contributors. All rights reserved.
#
# SPDX-License-Identifier: Apache-2.0

.PHONY: gotools-install
gotools-install:
	go get -u github.com/AlekSi/gocov-xml
	go get -u github.com/axw/gocov/gocov
	go get -u github.com/client9/misspell/cmd/misspell
	go get -u github.com/golang/protobuf/protoc-gen-go@b5d812f8a3706043e23a9cd5babf2e5423744d30
	go get -u github.com/maxbrunsfeld/counterfeiter/v6
	go get -u github.com/onsi/ginkgo/ginkgo@eea6ad008b96acdaa524f5b409513bf062b500ad
	go get -u github.com/vektra/mockery/cmd/mockery
	go get -u golang.org/x/lint/golint@959b441ac422379a43da2230f62be024250818b0
	go get -u golang.org/x/tools/cmd/goimports
