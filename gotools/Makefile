# Copyright London Stock Exchange Group All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

BINDIR ?= /usr/local/bin
OBJDIR ?= build

TMP_GOPATH=$(OBJDIR)/gopath
GOBIN=$(abspath $(TMP_GOPATH)/bin)

GOTOOLS = dep golint goimports protoc-gen-go ginkgo gocov gocov-xml misspell
GOTOOLS_BIN = $(patsubst %,$(GOBIN)/%, $(GOTOOLS))

# go tool->path mapping
go.fqp.dep       := github.com/golang/dep/cmd/dep
go.fqp.golint    := github.com/golang/lint/golint
go.fqp.goimports := golang.org/x/tools/cmd/goimports
go.fqp.gocov     := github.com/axw/gocov/...
go.fqp.misspell  := github.com/client9/misspell/cmd/misspell
go.fqp.gocov-xml:= github.com/AlekSi/gocov-xml
all: $(GOTOOLS_BIN)

install: $(GOTOOLS_BIN)
	@mkdir -p $(BINDIR)
	cp $^ $(BINDIR)

# Special override for protoc-gen-go since we want to use the version vendored with the project
gotool.protoc-gen-go:
	@echo "Building github.com/golang/protobuf/protoc-gen-go -> protoc-gen-go"
	@mkdir -p $(TMP_GOPATH)/src/github.com/golang/protobuf/
	@cp -R $(GOPATH)/src/github.com/hyperledger/fabric/vendor/github.com/golang/protobuf/* $(TMP_GOPATH)/src/github.com/golang/protobuf
	@GOPATH=$(abspath $(TMP_GOPATH)) go install github.com/golang/protobuf/protoc-gen-go

# Special override for ginkgo since we want to use the version vendored with the project
gotool.ginkgo:
	@echo "Building github.com/onsi/ginkgo/ginkgo -> ginkgo"
	@mkdir -p $(TMP_GOPATH)/src/github.com/onsi/ginkgo/ginkgo/
	@cp -R $(GOPATH)/src/github.com/hyperledger/fabric/vendor/github.com/onsi/ginkgo/* $(TMP_GOPATH)/src/github.com/onsi/ginkgo
	@GOPATH=$(abspath $(TMP_GOPATH)) go install github.com/onsi/ginkgo/ginkgo

# Lock to a versioned dep
gotool.dep: DEP_VERSION ?= "0.4.1"
gotool.dep:
	@echo "Getting dep $(DEP_VERSION)"
	@GOPATH=$(abspath $(TMP_GOPATH)) go get -d github.com/golang/dep
	@cd $(abspath $(TMP_GOPATH)) git checkout $(DEP_VERSION)
	@echo "Building github.com/golang/dep -> dep"
	@GOPATH=$(abspath $(TMP_GOPATH)) go install -ldflags="-X main.version=$(DEP_VERSION) -X main.buildDate=$$(date '+%Y-%m-%d')" github.com/golang/dep/cmd/dep

# Default rule for gotools uses the name->path map for a generic 'go get' style build
gotool.%:
	$(eval TOOL = ${subst gotool.,,${@}})
	@echo "Building ${go.fqp.${TOOL}} -> $(TOOL)"
	@GOPATH=$(abspath $(TMP_GOPATH)) go get ${go.fqp.${TOOL}}

$(GOBIN)/%:
	$(eval TOOL = ${subst $(GOBIN)/,,${@}})
	@$(MAKE) gotool.$(TOOL)

.PHONY: clean
clean:
	-@rm -rf $(OBJDIR)
