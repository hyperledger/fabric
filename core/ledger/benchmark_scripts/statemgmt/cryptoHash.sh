#!/bin/bash
source ../common.sh

PKG_PATH="github.com/hyperledger/fabric/core/ledger/statemgmt"
FUNCTION_NAME="BenchmarkCryptoHash"
NUM_CPUS=1
CHART_DATA_COLUMN="Number of Bytes"

setupAndCompileTest

for i in 1 5 10 20 50 100 200 400 600 800 1000 2000 5000 10000 20000 50000 100000; do
  TEST_PARAMS="-NumBytes=$i"
  CHART_COLUMN_VALUE=$i
  executeTest
done

constructChart
