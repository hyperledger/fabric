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

EXECUTABLES = npm
K := $(foreach exec,$(EXECUTABLES),\
	$(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH: Check dependencies")))

all: hfc

.PHONY: hfc
hfc:
	cp ../../protos/*.proto ./lib/protos
	cp ../../membersrvc/protos/*.proto ./lib/protos
	npm install && sudo npm install -g typescript && sudo npm install typings --global && typings install
	tsc
	./makedoc.sh

unit-tests: hfc
	@./bin/run-unit-tests.sh

clean:
