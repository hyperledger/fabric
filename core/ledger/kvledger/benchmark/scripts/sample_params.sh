#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

# OUTPUT_DIR_ROOT is the folder where the experiments results are persisted
OUTPUT_DIR_ROOT="/tmp/fabric/ledgersPerfTests/results"
# DataDir is used as ledger data folder
DataDir="/tmp/fabric/ledgersPerfTests/data"
export useCouchDB="false"
UseJSONFormat="false"
NumChains=10
NumParallelTxPerChain=10
NumKVs=10000
NumTotalTx=10000
NumWritesPerTx=4
NumReadsPerTx=4
BatchSize=50
KVSize=200

#####################################################################################################################
# Following variables controls what experiments to run. Typically, you would wish to run only selected experiments. 
# Comment out the variables that you do not want to run experiments for
#####################################################################################################################

# Whether to run experiment with default params above (see function 'shrunWithDefaultParams' in file runbenchmarks.sh)
RunWithDefaultParams=true
# Run experiments with varying "NumParallelTxPerChain" (keeping remaining params as default - see function 'varyNumParallelTxPerChain' in file runbenchmarks.sh)
ArrayNumParallelTxPerChain=(1 5 10 20 50 100 500 2000)
# Run experiments with varying "NumChains" (keeping remaining params as default - see function 'varyNumChains' in file runbenchmarks.sh)
ArrayNumChains=(1 5 10 20 50 100 500 2000)
# Run experiments with varying "NumWritesPerTx" (keeping remaining params as default - see function 'varyNumWritesPerTx' in file runbenchmarks.sh)
ArrayNumWritesPerTx=(1 2 5 10 20)
# Run experiments with varying "NumReadsPerTx" (keeping remaining params as default - see function 'varyNumReadsPerTx' in file runbenchmarks.sh)
ArrayNumReadsPerTx=(1 2 5 10 20)
# Run experiments with varying "KVSize" (keeping remaining params as default - see function 'varyKVSize' in file runbenchmarks.sh)
ArrayKVSize=(100 200 500 1000 2000)
# Run experiments with varying "BatchSize" (keeping remaining params as default - see function 'varyBatchSize' in file runbenchmarks.sh)
ArrayBatchSize=(10 20 100 500)
# Run experiments with varying "NumTotalTx" (keeping remaining params as default - see function 'varyNumTxs' in file runbenchmarks.sh)
ArrayNumTxs=(100000 200000 500000 1000000)
# Whether to run experiment with large amount of data (see function 'runLargeDataExperiment' in file runbenchmarks.sh)
RunLargeDataExperiment=true