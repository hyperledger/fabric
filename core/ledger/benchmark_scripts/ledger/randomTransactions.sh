#!/bin/bash
source ../common.sh


PKG_PATH="github.com/hyperledger/fabric/core/ledger"
NUM_CPUS=4
CHART_DATA_COLUMN="KVSize"

compileTest
OUTPUT_DIR="BenchmarkLedgerRandomTransactions"
createOutputDir
CHART_DATA_COLUMN="TEST_PARAMS"
writeBenchmarkHeader

function populateDB {
  FUNCTION_NAME="BenchmarkLedgerPopulate"
  TEST_PARAMS="-KeyPrefix=$KeyPrefix, -KVSize=$KVSize, -BatchSize=$BatchSize, -MaxKeySuffix=$MaxKeySuffix"
  CHART_COLUMN_VALUE="POPULATE_DB:Type=$LEDGER_STATE_DATASTRUCTURE_NAME:KeyPrefix=$KeyPrefix:KVSize=$KVSize:BatchSize=$BatchSize:MaxKeySuffix=$MaxKeySuffix:TestNumber=$TestNumber"
  executeTest
}

function runRandomTransactions {
  FUNCTION_NAME="BenchmarkLedgerRandomTransactions"
  TEST_PARAMS="-KeyPrefix=$KeyPrefix, -KVSize=$KVSize, -BatchSize=$BatchSize, -MaxKeySuffix=$MaxKeySuffix, -NumBatches=$NumBatches, -NumReadsFromLedger=$NumReadsFromLedger, -NumWritesToLedger=$NumWritesToLedger"
  CHART_COLUMN_VALUE="RANDOM_TRANSACTION_EXE:Type=$LEDGER_STATE_DATASTRUCTURE_NAME:KeyPrefix=$KeyPrefix:KVSize=$KVSize:BatchSize=$BatchSize:MaxKeySuffix=$MaxKeySuffix:NumBatches=$NumBatches:NumReadsFromLedger=$NumReadsFromLedger:NumWritesToLedger=$NumWritesToLedger:TestNumber=$TestNumber"
  executeTest
}

function initDBPath {
  DB_DIR="BenchmarkLedgerRandomTransactions/TestNumber=$TestNumber"
  configureDBPath
}

function runTest {
  initDBPath
  populateDB
  if [ "$CLEAR_OS_CACHE" == "true" ]; then
    clearOSCache
  fi
  runRandomTransactions
}

KeyPrefix=key_
MaxKeySuffix=1000000

export LEDGER_STATE_DATASTRUCTURE_NAME="buckettree"

# Before performing any of the following tests, manually delete the following folders from previous runs (if any)
# ~/obc_perf/db (Contains the db from the test run)
# ~/obc_perf/output/ledger (Contains the output from the test run)

##################     Measure the effect of bucket-cache START    ############################
# For enabling cache -
# 1) Change the value of 0 of 'bucketCacheSize' in test.yaml to 100
# 2) Comment the following three lines and uncomment the next three lines

TestNumber=1;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=1;runTest
TestNumber=2;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=4;runTest
TestNumber=3;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=4;NumWritesToLedger=1;runTest

#TestNumber=4;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=1;runTest
#TestNumber=5;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=4;runTest
#TestNumber=6;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=4;NumWritesToLedger=1;runTest
##################     Measure the effect of bucket-cache END    ############################

: '
################### Compare with raw state implementation  START   ############################
CLEAR_OS_CACHE=false
export LEDGER_STATE_DATASTRUCTURE_NAME="raw"
TestNumber=1;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=1;runTest
TestNumber=2;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=4;runTest
TestNumber=3;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=4;NumWritesToLedger=1;runTest

export LEDGER_STATE_DATASTRUCTURE_NAME="buckettree"
TestNumber=4;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=1;runTest
TestNumber=5;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=4;runTest
TestNumber=6;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=4;NumWritesToLedger=1;runTest

CLEAR_OS_CACHE=true
export LEDGER_STATE_DATASTRUCTURE_NAME="raw"
TestNumber=7;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=1;runTest
TestNumber=8;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=4;runTest
TestNumber=9;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=4;NumWritesToLedger=1;runTest

export LEDGER_STATE_DATASTRUCTURE_NAME="buckettree"
TestNumber=10;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=1;runTest
TestNumber=11;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=1;NumWritesToLedger=4;runTest
TestNumber=12;KVSize=100;BatchSize=100;NumBatches=1000;NumReadsFromLedger=4;NumWritesToLedger=1;runTest
################### Compare with raw state implementation  END   ############################
'
