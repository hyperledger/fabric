#!/bin/bash
# Copyright the Hyperledger Fabric contributors. All rights reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

sudo apt-get install -y softhsm2
sudo mkdir -p /var/lib/softhsm/tokens
sudo softhsm2-util --init-token --slot 0 --label "ForFabric" --so-pin 1234 --pin 98765432
sudo chmod -R 777 /var/lib/softhsm
mkdir -p ~/.config/softhsm2
cp /usr/share/softhsm/softhsm2.conf ~/.config/softhsm2
