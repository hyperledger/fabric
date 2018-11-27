#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

cd $HOME
mkdir -p $HOME/lib/softhsm/tokens
cd $HOME/lib/softhsm/
echo "directories.tokendir = $PWD/tokens" > softhsm2.conf
echo "Update SOFTHSM2_CONF via export SOFTHSM2_CONF=$HOME/lib/softhsm/softhsm2.conf"

export SOFTHSM2_CONF=$HOME/lib/softhsm/softhsm2.conf

softhsm2-util --init-token --slot 0 --label "ForFabric" --so-pin 1234 --pin 98765432
