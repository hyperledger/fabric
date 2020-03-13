#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

COMPOSE_VERSION=1.24.0

export DEBIAN_FRONTEND=noninteractive

# ----------------------------------------------------------------
# Configure apt repository
# ----------------------------------------------------------------
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository \
  "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) \
  stable"

# ----------------------------------------------------------------
# Install docker
# ----------------------------------------------------------------
apt-get -qq update
apt-get install -y docker-ce

# ----------------------------------------------------------------
# Allow vagrant user to access docker
# ----------------------------------------------------------------
usermod -a -G docker vagrant

# ----------------------------------------------------------------
# Install docker-compose
# ----------------------------------------------------------------
curl -sL "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-$(uname -s)-$(uname -m)" > /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose
