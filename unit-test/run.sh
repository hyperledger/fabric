#!/bin/bash

set -e
ARCH=`uname -m`

echo -n "Obtaining list of tests to run.."
# Some examples don't play nice with `go test`
PKGS=`go list github.com/hyperledger/fabric/... 2> /dev/null | \
                                                  grep -v /vendor/ | \
                                                  grep -v /build/ | \
	                                          grep -v /examples/chaincode/chaintool/ | \
						  grep -v /examples/chaincode/go/asset_management | \
						  grep -v /examples/chaincode/go/utxo | \
						  grep -v /examples/chaincode/go/rbac_tcerts_no_attrs`

if [ x$ARCH == xppc64le -o x$ARCH == xs390x ]
then
PKGS=`echo $PKGS | sed  's@'github.com/hyperledger/fabric/core/chaincode/platforms/java/test'@@g'`
PKGS=`echo $PKGS | sed  's@'github.com/hyperledger/fabric/core/chaincode/platforms/java'@@g'`
fi

echo "DONE!"

echo "Running tests..."
gocov test -ldflags "$GO_LDFLAGS" $PKGS -p 1 -timeout=20m | gocov-xml > report.xml

