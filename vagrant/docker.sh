#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

export DEBIAN_FRONTEND=noninteractive

# ----------------------------------------------------------------
# Install docker
# ----------------------------------------------------------------
apt-get -qq update
apt-get install -y docker.io

# ----------------------------------------------------------------
# Allow vagrant user to access docker
# ----------------------------------------------------------------
usermod -a -G docker vagrant
