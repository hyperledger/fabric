#!/bin/bash`
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

export useCouchDB="yes"
UseJSONFormat="true"
DataDir="/tmp/fabric/test/tools/LTE/data"
NumChains=10
NumParallelTxPerChain=10
NumKVs=10000
NumTotalTx=10000
NumWritesPerTx=4
NumReadsPerTx=4
BatchSize=50
KVSize=200

# Each test consists of several test-runs, where one single parameter is varied
# between the test-runs and rest of the parameters remain same. Each array below
# holds the values of that parameter for each corresponding test. For example,
# the "ArrayNumChains" will be used in the test titled "varyNumChains", and the
# 8 test-runs that exist for testing the varying number of chains will use these
# values:
# NumChains = (1 5 10 20 50 100 500 2000)
# NumParallelTxPerChain=10
# NumKVs=10000
# NumTotalTx=10000
# NumWritesPerTx=4
# BatchSize=50
# KVSize=200
ArrayNumParallelTxPerChain=(1 5 10 20 50 100)
ArrayNumChains=(1 5 10 20 50)
ArrayNumWritesPerTx=(1 2 5 10 20)
ArrayKVSize=(100 200 500 1000 2000)
ArrayBatchSize=(10 20 100 500)
ArrayNumParallelTxWithSingleChain=(1 5 10 20 50 100)
ArrayNumChainsWithNoParallelism=(1 5 10 20 50)
ArrayNumTxs=(10000 20000 50000 100000)
