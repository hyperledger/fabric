#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e
ARCH=`uname -m`

#check job type, do patch set specific unit test when job is verify
if [ "$JOB_TYPE"  = "VERIFY" ]; then

  cd $GOPATH/src/github.com/hyperledger/fabric/

  #figure out what packages should be tested for uncommitted changes
  # first check for uncommitted changes
  TEST_PKGS=$(git diff --name-only HEAD * | grep .go$ | grep -v ^vendor/ \
    | grep -v ^build/ | sed 's%/[^/]*$%/%'| sort -u \
    | awk '{print "github.com/hyperledger/fabric/"$1"..."}')

  if [ -z "$TEST_PKGS" ]; then
    # next check for changes in the latest commit - typically this will
    # be for CI only, but could also handle a committed change before
    # pushing to Gerrit
    TEST_PKGS=$(git diff-tree --no-commit-id --name-only -r $(git log -2 \
      --pretty=format:"%h") | grep .go$ | grep -v ^vendor/ | grep -v ^build/ \
      | sed 's%/[^/]*$%/%'| sort -u | \
      awk '{print "github.com/hyperledger/fabric/"$1"..."}')
  fi

  #only run the test when test pkgs is not empty
  if [[ ! -z "$TEST_PKGS" ]]; then
     echo "Testing packages:"
     echo $TEST_PKGS
     # use go test -cover as this is much more efficient than gocov
     time go test -cover -ldflags "$GO_LDFLAGS" $TEST_PKGS -p 1 -timeout=20m
  else
     echo "Nothing changed in unit test!!!"
  fi

else

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

echo " DONE!"

echo "Running tests..."
#go test -cover -ldflags "$GO_LDFLAGS" $PKGS -p 1 -timeout=20m
gocov test -ldflags "$GO_LDFLAGS" $PKGS -p 1 -timeout=20m | gocov-xml > report.xml
fi
