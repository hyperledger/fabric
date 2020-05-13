#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

export DEBIAN_FRONTEND=noninteractive

# ----------------------------------------------------------------
# Update the entire system to the latest versions
# ----------------------------------------------------------------
apt-get clean && apt-get -qq update && apt-get upgrade -y

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
    jq \
    make \
    unzip
