#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

DAILYDIR="$GOPATH/src/github.com/hyperledger/fabric/test/regression/daily"

#echo "========== Sample Tests..."
#py.test -v --junitxml results_sample.xml Example.py

echo "========== System Test Performance Stress tests driven by PTE tool..."
py.test -v --junitxml results_systest_pte.xml systest_pte.py

echo "========== Behave feature and system tests..."
cd ../../feature
behave --junit --junit-directory ../regression/daily/. --tags=-skip --tags=daily
cd -

echo "========== Ledger component performance tests..."
py.test -v --junitxml results_ledger_lte.xml ledger_lte.py

echo "========== Test Auction Chaincode ..."
py.test -v --junitxml results_auction_daily.xml testAuctionChaincode.py
