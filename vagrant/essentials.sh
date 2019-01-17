#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

# ----------------------------------------------------------------
# Update the entire system to the latest releases
# ----------------------------------------------------------------
apt-get update

# ----------------------------------------------------------------
# Install some basic utilities
# ----------------------------------------------------------------
apt-get install -y \
    apt-transport-https \
    build-essential \
    ca-certificates \
    curl \
    g++ \
    git \
    libtool \
    make \
    unzip
