#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -e

ARCH=`uname -m`

if [ $ARCH = "s390x" ]; then
  echo "deb http://ftp.us.debian.org/debian sid main" >> /etc/apt/sources.list
fi

# Install softhsm2 package
apt-get update
apt-get -y install softhsm2

# Create tokens directory
mkdir -p /var/lib/softhsm/tokens/

#Initialize token
softhsm2-util --init-token --slot 0 --label "ForFabric" --so-pin 1234 --pin 98765432
