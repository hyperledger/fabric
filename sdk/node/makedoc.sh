#!/bin/bash

# -------------------------------------------------------------
#
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
# -------------------------------------------------------------

typedoc=$(which typedoc)
if [[ $? -ne 0 ]]; then
    echo "Installing typedoc ..."
    sudo npm install -g typedoc
    if [[ $? -ne 0 ]]; then
       echo "No typedoc found. Please install it like this:"
       echo "  npm install -g typedoc"
       echo "and rerun this shell script again."
       exit 1
    fi
    echo "Successfully installed typedoc"
fi
set -e

tv="$(typings -v)"
if [[ "${tv%.*}" < "1.0" ]]; then
    echo "You have typings ${tv} but you need 1.0 or higher."
    exit 1
fi

mkdir -p doc
rm -rf doc/*

typedoc -m amd \
--name 'Node.js Hyperledger Fabric SDK' \
--includeDeclarations \
--excludeExternals \
--excludeNotExported \
--out doc \
src/hfc.ts typedoc-special.d.ts

# Typedoc generates links to working GIT repo which is fixed
# below to use an official release URI.
DOCURI="https://github.com/hyperledger/fabric/tree/master/sdk/node/"
find doc -name '*.html' -exec sed -i 's!href="http.*sdk/node/!href="'$DOCURI'!' {} \;
