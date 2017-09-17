#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e
set -x

# ----------------------------------------------------------------
# Install nvm to manage multiple NodeJS versions
# ----------------------------------------------------------------
curl -o- https://raw.githubusercontent.com/creationix/nvm/v0.33.4/install.sh | bash
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && . "$NVM_DIR/nvm.sh" # This loads nvm

# ----------------------------------------------------------------
# Install NodeJS
# ----------------------------------------------------------------
nvm install v6.9.5
nvm install v8.4
nvm alias default v8.4 #set default to v8.4
