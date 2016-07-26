#!/bin/bash
source ../common.sh

PKG_PATH="github.com/hyperledger/fabric/core/ledger/statemgmt/buckettree"
FUNCTION_NAME="BenchmarkStateHash"
NUM_CPUS=1
CHART_DATA_COLUMN="NUM EXISTING KEYS"
export PEER_LEDGER_TEST_LOADYAML=false

function runTest {
  OUTPUT_DIR="$FUNCTION_NAME/${NumBuckets}_${KVSize}"
  DB_DIR="$FUNCTION_NAME/${NumBuckets}_${KVSize}"
  TEST_PARAMS="-NumBuckets=$NumBuckets,\
        -MaxGroupingAtEachLevel=$MaxGroupingAtEachLevel,\
        -ChaincodeIDPrefix=$ChaincodeIDPrefix,\
        -NumChaincodes=$NumChaincodes,\
        -MaxKeySuffix=$MaxKeySuffix,\
        -NumKeysToInsert=$NumKeysToInsert,\
        -KVSize=$KVSize"

  setupAndCompileTest

  for i in `seq 0 999`; do
    EXISTING_KEYS_IN_DB=$(($i*$NumKeysToInsert))
    echo "executing with existing keys=$EXISTING_KEYS_IN_DB"
    CHART_COLUMN_VALUE=$EXISTING_KEYS_IN_DB
    executeTest
  done

  ADDITIONAL_TEST_FLAGS="-test.cpuprofile=cpu.out -test.outputdir=`getOuputDir`"
  CHART_COLUMN_VALUE=$(($(($i+1))*$NumKeysToInsert))
  executeTest
  constructChart
}

##### TEST PARAMS
MaxGroupingAtEachLevel=5
ChaincodeIDPrefix="chaincode"
NumChaincodes=5
MaxKeySuffix=1000000
NumKeysToInsert=1000

NumBuckets=1009;KVSize=20;runTest
NumBuckets=10009;KVSize=20;runTest
NumBuckets=100003;KVSize=20;runTest
NumBuckets=1000003;KVSize=20;runTest

NumBuckets=1009;KVSize=50;runTest
NumBuckets=10009;KVSize=50;runTest
NumBuckets=100003;KVSize=50;runTest
NumBuckets=1000003;KVSize=50;runTest

NumBuckets=1009;KVSize=100;runTest
NumBuckets=10009;KVSize=100;runTest
NumBuckets=100003;KVSize=100;runTest
NumBuckets=1000003;KVSize=100;runTest

NumBuckets=1009;KVSize=300;runTest
NumBuckets=10009;KVSize=300;runTest
NumBuckets=100003;KVSize=300;runTest
NumBuckets=1000003;KVSize=300;runTest

NumBuckets=1009;KVSize=500;runTest
NumBuckets=10009;KVSize=500;runTest
NumBuckets=100003;KVSize=500;runTest
NumBuckets=1000003;KVSize=500;runTest

NumBuckets=1009;KVSize=1000;runTest
NumBuckets=10009;KVSize=1000;runTest
NumBuckets=100003;KVSize=1000;runTest
NumBuckets=1000003;KVSize=1000;runTest

NumBuckets=1009;KVSize=2000;runTest
NumBuckets=10009;KVSize=2000;runTest
NumBuckets=100003;KVSize=2000;runTest
NumBuckets=1000003;KVSize=2000;runTest

NumBuckets=1009;KVSize=5000;runTest
NumBuckets=10009;KVSize=5000;runTest
NumBuckets=100003;KVSize=5000;runTest
NumBuckets=1000003;KVSize=5000;runTest
