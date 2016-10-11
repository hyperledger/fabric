#!/bin/bash

set -e

TAG=$1
GO_LDFLAGS=$2

BASEIMAGE="hyperledger/fabric-peer"
IMAGE=$BASEIMAGE
ARCH=`uname -m`

if [ "$TAG" != "" ]
then
    IMAGE="$BASEIMAGE:$TAG"
fi

echo "Running unit tests using $IMAGE"

echo "Cleaning membership services folder"
rm -rf membersrvc/ca/.ca/

echo -n "Obtaining list of tests to run.."
# Some examples don't play nice with `go test`
PKGS=`go list github.com/hyperledger/fabric/... | grep -v /vendor/ | \
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

echo -n "Starting peer.."
CID=`docker run -dit -p 7051:7051 $IMAGE peer node start`
cleanup() {
    echo "Stopping peer.."
    docker kill $CID 2>&1 > /dev/null
}
trap cleanup 0
echo "DONE!"

echo "Running tests..."
gocov test -ldflags "$GO_LDFLAGS" $PKGS -p 1 -timeout=20m | gocov-xml > report.xml
