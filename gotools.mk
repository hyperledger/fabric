# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

GOTOOLS = counterfeiter ginkgo gocov gocov-xml goimports golint misspell mockery protoc-gen-go
BUILD_DIR ?= build
GOTOOLS_BINDIR ?= $(shell go env GOPATH)/bin

# go tool->path mapping
go.fqp.counterfeiter := github.com/maxbrunsfeld/counterfeiter/v6
go.fqp.ginkgo        := github.com/onsi/ginkgo/ginkgo
go.fqp.gocov         := github.com/axw/gocov/gocov
go.fqp.gocov-xml     := github.com/AlekSi/gocov-xml
go.fqp.goimports     := golang.org/x/tools/cmd/goimports
go.fqp.golint        := golang.org/x/lint/golint
go.fqp.misspell      := github.com/client9/misspell/cmd/misspell
go.fqp.mockery       := github.com/vektra/mockery/cmd/mockery
go.fqp.protoc-gen-go := github.com/golang/protobuf/protoc-gen-go

.PHONY: gotools-install
gotools-install: $(patsubst %,$(GOTOOLS_BINDIR)/%, $(GOTOOLS))

.PHONY: gotools-clean
gotools-clean:

# Default rule for gotools uses the name->path map for a generic 'go get' style build
gotool.%:
	$(eval TOOL = ${subst gotool.,,${@}})
	@echo "Building ${go.fqp.${TOOL}} -> $(TOOL)"
	@cd tools && GO111MODULE=on GOBIN=$(abspath $(GOTOOLS_BINDIR)) go install ${go.fqp.${TOOL}}

$(GOTOOLS_BINDIR)/%:
	$(eval TOOL = ${subst $(GOTOOLS_BINDIR)/,,${@}})
	@$(MAKE) -f gotools.mk gotool.$(TOOL)
