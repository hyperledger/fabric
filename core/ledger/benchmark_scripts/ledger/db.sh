#!/bin/bash
source ../common.sh

PKG_PATH="github.com/hyperledger/fabric/core/ledger"
FUNCTION_NAME="BenchmarkDB"
NUM_CPUS=1
CHART_DATA_COLUMN="Number of Bytes"

setupAndCompileTest

KVSize=1000
MaxKeySuffix=1000000
KeyPrefix=ChaincodeKey_

CHART_COLUMN_VALUE=$KVSize

## now populate the db with 'MaxKeySuffix' number of key-values
TEST_PARAMS="-KVSize=$KVSize, -PopulateDB=true, -MaxKeySuffix=$MaxKeySuffix, -KeyPrefix=$KeyPrefix"
executeTest

# now perform random access test. If you want to perform the random access test
TEST_PARAMS="-KVSize=$KVSize, -PopulateDB=false, -MaxKeySuffix=$MaxKeySuffix, -KeyPrefix=$KeyPrefix"
executeTest

# now perform random access test after clearing OS file-system cache. If you want to perform the random access test
clearOSCache
executeTest
