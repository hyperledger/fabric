#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

source ./benchmarks.sh
source ./parameters_daily_CI.sh

########################################################################################################
# This shell script contains a series of benchmark tests
# The test parameters are imported from "parameters_daily_CI.sh"
########################################################################################################

function varyNumParallelTxPerChain {
    for v in 1 5 10 20 50 100 500 2000; do
        NumParallelTxPerChain=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumChains {
    for v in 1 5 10 20 50 100 500 2000; do
        NumChains=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumKeysInEachTx {
    for v in 1 2 5 10 20; do
        NumKeysInEachTx=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyKVSize {
    for v in 100 200 500 1000 2000; do
        KVSize=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyBatchSize {
    for v in 10 20 100 500; do
        BatchSize=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumParallelTxWithSingleChain {
    NumChains=1
    for v in 1 5 10 20 50 100 500 2000; do
        NumParallelTxPerChain=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumChainsWithNoParallelism {
    NumParallelTxPerChain=1
    for v in 1 5 10 20 50 100 500 2000; do
        NumChains=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumTxs {
    for v in 1000000 2000000 5000000 10000000; do
        NumTotalTx=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function runLargeDataExperiment {
    NumKVs=10000000
    NumTotalTx=10000000
    rm -rf $DataDir;runInsertTxs;runReadWriteTxs
}

shift $(expr $OPTIND - 1 )

case $1 in
  varyNumParallelTxPerChain)
    varyNumParallelTxPerChain ;;
  varyNumChains)
    varyNumChains ;;
  varyNumParallelTxWithSingleChain)
    varyNumParallelTxWithSingleChain ;;
  varyNumChainsWithNoParallelism)
    varyNumChainsWithNoParallelism ;;
  varyNumKeysInEachTx)
    varyNumKeysInEachTx ;;
  varyKVSize)
    varyKVSize ;;
  varyBatchSize)
    varyBatchSize ;;
  varyNumTxs)
    varyNumTxs ;;
  runLargeDataExperiment)
    runLargeDataExperiment ;;
  help)
    printf "Usage: ./runbenchmarks.sh [test_name]\nAvailable tests (use \"all\" to run all tests):
varyNumParallelTxPerChain
varyNumChains
varyNumParallelTxWithSingleChain
varyNumChainsWithNoParallelism
varyNumKeysInEachTx
varyKVSize
varyBatchSize
varyNumTxs
runLargeDataExperiment\n"
    ;;
  all)
    varyNumParallelTxPerChain
    varyNumChains
    varyNumParallelTxWithSingleChain
    varyNumChainsWithNoParallelism
    varyNumKeysInEachTx
    varyKVSize
    varyBatchSize
    varyNumTxs
    runLargeDataExperiment ;;
  *)
    printf "Error: test name empty/incorrect!\n"  >> /dev/stderr
    exit 1 ;;
esac
