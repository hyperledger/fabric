# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

GOTOOLS = counterfeiter gendoc ginkgo gocov gocov-xml golangci-lint govulncheck mockery protoc-gen-go protoc-gen-go-grpc swagger
BUILD_DIR ?= build
GOTOOLS_BINDIR ?= $(shell go env GOPATH)/bin

# go tool->path mapping
go.fqp.counterfeiter      := github.com/maxbrunsfeld/counterfeiter/v6
go.fqp.gendoc		      := github.com/hyperledger/fabric-lib-go/common/metrics/cmd/gendoc
go.fqp.ginkgo             := github.com/onsi/ginkgo/v2/ginkgo
go.fqp.gocov              := github.com/axw/gocov/gocov
go.fqp.gocov-xml          := github.com/AlekSi/gocov-xml
go.fqp.golangci-lint      := github.com/golangci/golangci-lint/v2/cmd/golangci-lint
go.fqp.mockery            := github.com/vektra/mockery/v2
go.fqp.protoc-gen-go      := google.golang.org/protobuf/cmd/protoc-gen-go
go.fqp.protoc-gen-go-grpc := google.golang.org/grpc/cmd/protoc-gen-go-grpc
go.fqp.swagger            := github.com/go-swagger/go-swagger/cmd/swagger

.PHONY: gotools-install
gotools-install: $(patsubst %,$(GOTOOLS_BINDIR)/%, $(GOTOOLS))

.PHONY: gotools-clean
gotools-clean:

# Default rule for gotools uses the name->path map for a generic 'go get' style build
gotool.%:
	$(eval TOOL = ${subst gotool.,,${@}})
	@echo "Building ${go.fqp.${TOOL}} -> $(TOOL)"
	@cd tools && GOBIN=$(abspath $(GOTOOLS_BINDIR)) go install ${go.fqp.${TOOL}}

$(GOTOOLS_BINDIR)/%:
	$(eval TOOL = ${subst $(GOTOOLS_BINDIR)/,,${@}})
	@$(MAKE) -f gotools.mk gotool.$(TOOL)
