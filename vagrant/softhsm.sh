#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

export DEBIAN_FRONTEND=noninteractive

# ----------------------------------------------------------------
# Install SoftHSM
# ----------------------------------------------------------------
apt-get install -y softhsm2

# ----------------------------------------------------------------
# Create tokens directory
# ----------------------------------------------------------------
mkdir -p /home/vagrant/.tokens
chown -R vagrant.vagrant /home/vagrant/.tokens

# ----------------------------------------------------------------
# Setup softhsm configuration file
# ----------------------------------------------------------------
mkdir -p /home/vagrant/.config/softhsm2
sed s,/var/lib/softhsm/tokens/,/home/vagrant/.tokens,g < /etc/softhsm/softhsm2.conf > /home/vagrant/.config/softhsm2/softhsm2.conf
chown -R vagrant.vagrant /home/vagrant/.config
