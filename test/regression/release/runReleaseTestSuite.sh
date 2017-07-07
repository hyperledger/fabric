#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

DAILYDIR="$GOPATH/src/github.com/hyperledger/fabric/test/regression/daily"
RELEASEDIR="$GOPATH/src/github.com/hyperledger/fabric/test/regression/release"

export FABRIC_ROOT_DIR=$GOPATH/src/github.com/hyperledger/fabric

cd $FABRIC_ROOT_DIR || exit

IS_RELEASE=`cat Makefile | grep IS_RELEASE | awk '{print $3}'`
echo "=======>" $IS_RELEASE

# IS_RELEASE=True specify the Release check. Trigger Release tests only
# if IS_RELEASE=TRUE

if [ $IS_RELEASE != "true" ]; then
echo "=======> TRIGGER ONLY on RELEASE !!!!!"
exit 0
else

cd $RELEASEDIR

docker rm -f $(docker ps -aq) || true
echo "=======> Execute make targets"
chmod +x run_make_targets.sh
py.test -v --junitxml results_make_targets.xml make_targets_release_tests.py

echo "=======> Execute SDK tests..."
chmod +x run_e2e_node_sdk.sh
chmod +x run_e2e_java_sdk.sh
py.test -v --junitxml results_e2e_sdk.xml e2e_sdk_release_tests.py

docker rm -f $(docker ps -aq) || true
echo "=======> Execute byfn tests..."
chmod +x run_byfn_cli_release_tests.sh
chmod +x run_node_sdk_byfn.sh
py.test -v --junitxml results_byfn_cli.xml byfn_release_tests.py

cd $DAILYDIR

docker rm -f $(docker ps -aq) || true
echo "=======> Ledger component performance tests..."
py.test -v --junitxml results_ledger_lte.xml ledger_lte.py

docker rm -f $(docker ps -aq) || true
echo "=======> Test Auction Chaincode ..."
py.test -v --junitxml results_auction_daily.xml testAuctionChaincode.py

fi
