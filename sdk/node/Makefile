# Licensed to the Apache Software Foundation (ASF) under one
# or more contributor license agreements.  See the NOTICE file
# distributed with this work for additional information
# regarding copyright ownership.  The ASF licenses this file
# to you under the Apache License, Version 2.0 (the
# "License"); you may not use this file except in compliance
# with the License.  You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.
#

BINDIR  := node_modules/.bin
TYPINGS := $(BINDIR)/typings
TSC     := $(BINDIR)/tsc
TYPEDOC := $(BINDIR)/typedoc

DOCURI := "https://github.com/hyperledger/fabric/tree/master/sdk/node/"

SRC := $(shell find src -type f)
CONFIG := $(shell ls *.json)

EXECUTABLES = npm
K := $(foreach exec,$(EXECUTABLES),\
	$(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH: Check dependencies")))

# npm tool->pkg mapping
npm.pkg.typings   := typings
npm.pkg.tsc       := typescript
npm.pkg.typedoc   := typedoc

all: lib doc

.PHONY: lib doc
lib: lib/hfc.js
doc: doc/index.html

.SECONDARY: $(TYPINGS) $(TSC) $(TYPEDOC)
$(BINDIR)/%: $(CONFIG)
	@echo "[INSTALL] $@"
	npm install $(npm.pkg.$(@F))
	@touch $@

doc/index.html: $(TYPEDOC) $(SRC) $(CONFIG) typedoc-special.d.ts Makefile
	@echo "[BUILD] SDK documentation"
	@-rm -rf $(@D)
	@mkdir -p $(@D)
	$(TYPEDOC) -m amd \
		--name 'Node.js Hyperledger Fabric SDK' \
		--includeDeclarations \
		--excludeExternals \
		--excludeNotExported \
		--out $(@D) \
		src/hfc.ts typedoc-special.d.ts
	# Typedoc generates links to working GIT repo which is fixed
	# below to use an official release URI.
	find $(@D) -name '*.html' -exec sed -i 's!href="http.*sdk/node/!href="'$(DOCURI)'!' {} \;

lib/hfc.js: $(TYPINGS) $(TSC) $(SRC) $(CONFIG) Makefile
	@echo "[BUILD] SDK"
	@mkdir -p ./lib/protos
	@cp ../../protos/*.proto ./lib/protos
	@cp ../../membersrvc/protos/*.proto ./lib/protos
	npm install
	$(TYPINGS) install
	$(TSC)

unit-tests: lib
	@echo "[RUN] unit tests..."
	@./bin/run-unit-tests.sh

clean:
	@echo "[CLEAN]"
	-rm -rf node_modules
	-rm -rf doc
	-find lib | grep -v "protos/google" | grep -v "hash.js" | xargs rm
