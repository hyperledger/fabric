#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

# ----------------------------------------------------------------
# Ensure synced folder path is accessible
# ----------------------------------------------------------------
find /home/vagrant/go -maxdepth 3 -exec chown vagrant:vagrant {} \;
