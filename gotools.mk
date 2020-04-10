# Copyright IBM Corp All Rights Reserved.
# Copyright London Stock Exchange Group All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

GOTOOLS = counterfeiter ginkgo gocov gocov-xml goimports golint misspell mockery protoc-gen-go
BUILD_DIR ?= build
GOTOOLS_BINDIR ?= $(shell go env GOPATH)/bin

# go tool->path mapping
go.fqp.counterfeiter := github.com/maxbrunsfeld/counterfeiter
go.fqp.gocov         := github.com/axw/gocov/gocov
go.fqp.gocov-xml     := github.com/AlekSi/gocov-xml
go.fqp.goimports     := golang.org/x/tools/cmd/goimports
go.fqp.golint        := golang.org/x/lint/golint
go.fqp.manifest-tool := github.com/estesp/manifest-tool
go.fqp.misspell      := github.com/client9/misspell/cmd/misspell
go.fqp.mockery       := github.com/vektra/mockery

.PHONY: gotools-install
gotools-install: $(patsubst %,$(GOTOOLS_BINDIR)/%, $(GOTOOLS))

.PHONY: gotools-clean
gotools-clean:
	-@rm -rf $(BUILD_DIR)/gotools

# Special override for protoc-gen-go since we want to use the version vendored with the project
gotool.protoc-gen-go:
	@echo "Building github.com/golang/protobuf/protoc-gen-go -> protoc-gen-go"
	GOBIN=$(abspath $(GOTOOLS_BINDIR)) go install ./vendor/github.com/golang/protobuf/protoc-gen-go

# Special override for ginkgo since we want to use the version vendored with the project
gotool.ginkgo:
	@echo "Building github.com/onsi/ginkgo/ginkgo -> ginkgo"
	GOBIN=$(abspath $(GOTOOLS_BINDIR)) go install ./vendor/github.com/onsi/ginkgo/ginkgo

# Special override for goimports since we want to use the version vendored with the project
gotool.goimports:
	@echo "Building golang.org/x/tools/cmd/goimports -> goimports"
	GOBIN=$(abspath $(GOTOOLS_BINDIR)) go install ./vendor/golang.org/x/tools/cmd/goimports

# Special override for golint since we want to use the version vendored with the project
gotool.golint:
	@echo "Building golang.org/x/lint/golint -> golint"
	GOBIN=$(abspath $(GOTOOLS_BINDIR)) go install ./vendor/golang.org/x/lint/golint

# Default rule for gotools uses the name->path map for a generic 'go get' style build
gotool.%:
	$(eval TOOL = ${subst gotool.,,${@}})
	@echo "Building ${go.fqp.${TOOL}} -> $(TOOL)"
	@GOPATH=$(abspath $(GOTOOLS_GOPATH)) GOBIN=$(abspath $(GOTOOLS_BINDIR)) go get ${go.fqp.${TOOL}}

$(GOTOOLS_BINDIR)/%:
	$(eval TOOL = ${subst $(GOTOOLS_BINDIR)/,,${@}})
	@$(MAKE) -f gotools.mk gotool.$(TOOL)
