#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

source ./benchmarks.sh

########################################################################################################
# This shell script invokes a series of benchmark tests with desired values of test specific parameters
########################################################################################################

function setDefaultTestParams {
    DataDir="/tmp/fabric/test/tools/ledgerbenchmarks/data"
    NumChains=10
    NumParallelTxPerChain=10
    NumKVs=1000000
    NumTotalTx=1000000
    NumKeysInEachTx=4
    BatchSize=50
    KVSize=200
}

function varyNumParallelTxPerChain {
    setDefaultTestParams
    for v in 1 5 10 20 50 100 500 2000; do
        NumParallelTxPerChain=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumChains {
    setDefaultTestParams
    for v in 1 5 10 20 50 100 500 2000; do
        NumChains=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumKeysInEachTx {
    setDefaultTestParams
    for v in 1 2 5 10 20; do
        NumKeysInEachTx=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyKVSize {
    setDefaultTestParams
    for v in 100 200 500 1000 2000; do
        KVSize=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyBatchSize {
    setDefaultTestParams
    for v in 10 20 100 500; do
        BatchSize=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumParallelTxWithSingleChain {
    setDefaultTestParams
    NumChains=1
    for v in 1 5 10 20 50 100 500 2000; do
        NumParallelTxPerChain=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumChainsWithNoParallelism {
    setDefaultTestParams
    NumParallelTxPerChain=1
    for v in 1 5 10 20 50 100 500 2000; do
        NumChains=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumTxs {
    setDefaultTestParams
    for v in 1000000 2000000 5000000 10000000; do
        NumTotalTx=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function runLargeDataExperiment {
    setDefaultTestParams
    NumKVs=10000000
    NumTotalTx=10000000
    rm -rf $DataDir;runInsertTxs;runReadWriteTxs
}

### Run tests
varyNumParallelTxPerChain
varyNumChains
varyNumParallelTxWithSingleChain
varyNumChainsWithNoParallelism
varyNumKeysInEachTx
varyKVSize
varyBatchSize
varyNumTxs
runLargeDataExperiment
