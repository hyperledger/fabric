#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

######################################################################
### Run one group of the tests in weekly test suite.

echo "========== Auction App 72Hr test"
py.test -v --junitxml results_auction_72Hr.xml testAuctionChaincode.py
