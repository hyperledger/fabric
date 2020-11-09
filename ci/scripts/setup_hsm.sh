#!/bin/bash
# Copyright the Hyperledger Fabric contributors. All rights reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

brew install softhsm
softhsm2-util --init-token --slot 0 --label "ForFabric" --so-pin 1234 --pin 98765432