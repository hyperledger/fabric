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

echo "========== Test Your Chaincode ..."
# TBD - after changeset https://gerrit.hyperledger.org/r/#/c/9163/ is merged,
# replace the previous 2 lines with this new syntax to run all the chaincode tests;
# and when making this change we should also remove file chaincodeTests/runChaincodes.sh)
#
#cd $DAILYDIR/chaincodeTests/envsetup
#py.test -v --junitxml ../../results_testYourChaincode.xml testYourChaincode.py

# TBD - after changeset https://gerrit.hyperledger.org/r/#/c/9251/ is merged,
# and integrated with this, lines like these should be executed too:
#echo "========== Ledger component performance tests..."
#cd $DAILYDIR/ledgerperftests
#py.test -v --junitxml results_perf_goleveldb.xml test_perf_goleveldb.py
#py.test -v --junitxml results_perf_couchdb.xml test_perf_couchdb.py
