#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

source ./common.sh

#######################################################################################################
# This shell script contains two functions that can be invoked to execute specific tests
#
# runInsertTxs - This function sets the environment variables and runs the benchmark function
# 'BenchmarkInsertTxs' in package 'github.com/hyperledger/fabric/test/tools/LTE/experiments'
#
# runReadWriteTxs - This function sets the environment variables and runs the benchmark function
# 'BenchmarkReadWriteTxs' in package 'github.com/hyperledger/fabric/test/tools/LTE/experiments'
#
# For the details of test specific parameters, refer to the documentation in 'go' files for the tests
#######################################################################################################

PKG_NAME="github.com/hyperledger/fabric/test/tools/LTE/experiments"

function setCommonTestParams {
  TEST_PARAMS="-DataDir=$DataDir, -NumChains=$NumChains, -NumParallelTxPerChain=$NumParallelTxPerChain, -NumKeysInEachTx=$NumKeysInEachTx, -BatchSize=$BatchSize, -NumKVs=$NumKVs, -KVSize=$KVSize"
  RESULTANT_DIRS="$DataDir/ledgersData/chains/chains $DataDir/ledgersData/chains/index $DataDir/ledgersData/stateLeveldb $DataDir/ledgersData/historyLeveldb"
}

function runInsertTxs {
  FUNCTION_NAME="BenchmarkInsertTxs"
  setCommonTestParams
  executeTest
}

function runReadWriteTxs {
  FUNCTION_NAME="BenchmarkReadWriteTxs"
  if [ "$CLEAR_OS_CACHE" == "true" ]; then
    clearOSCache
  fi    
  setCommonTestParams
  TEST_PARAMS="$TEST_PARAMS, -NumTotalTx=$NumTotalTx"
  executeTest
}
