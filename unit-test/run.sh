#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e
ARCH=`uname -m`

#check to see if TEST_PKGS is set else use default (all packages)
TEST_PKGS=${TEST_PKGS:-github.com/hyperledger/fabric/...}
echo -n "Obtaining list of tests to run for the following packages: ${TEST_PKGS}"

# Some examples don't play nice with `go test`
PKGS=`go list ${TEST_PKGS} 2> /dev/null | \
                                                  grep -v /vendor/ | \
                                                  grep -v /build/ | \
                                                  grep -v /bccsp/mocks | \
                                                  grep -v /bddtests | \
                                                  grep -v /orderer/mocks | \
                                                  grep -v /orderer/sample_clients | \
                                                  grep -v /common/mocks | \
                                                  grep -v /common/ledger/testutil | \
                                                  grep -v /core/mocks | \
                                                  grep -v /core/testutil | \
                                                  grep -v /core/ledger/testutil | \
                                                  grep -v /core/ledger/kvledger/example | \
                                                  grep -v /core/ledger/kvledger/marble_example | \
                                                  grep -v /core/deliverservice/mocks | \
                                                  grep -v /core/scc/samplesyscc | \
                                                  grep -v /test | \
                                                  grep -v /examples`

if [ x$ARCH == xppc64le -o x$ARCH == xs390x ]
then
PKGS=`echo $PKGS | sed  's@'github.com/hyperledger/fabric/core/chaincode/platforms/java/test'@@g'`
PKGS=`echo $PKGS | sed  's@'github.com/hyperledger/fabric/core/chaincode/platforms/java'@@g'`
fi

echo "DONE!"

echo "Running tests..."
#go test -cover -ldflags "$GO_LDFLAGS" $PKGS -p 1 -timeout=20m
gocov test -ldflags "$GO_LDFLAGS" $PKGS -p 1 -timeout=20m | gocov-xml > report.xml
