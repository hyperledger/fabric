#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

source ./benchmarks.sh

########################################################################################################
# This shell script contains a series of benchmark tests
########################################################################################################
function runWithDefaultParams {
  source $PARAM_FILE  
  if [[ $RunWithDefaultParams = "true" ]]
  then
    rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
  fi
}

function varyNumParallelTxPerChain {
    source $PARAM_FILE    
    for v in "${ArrayNumParallelTxPerChain[@]}"
    do
        NumParallelTxPerChain=$v
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
    done
}

function varyNumChains {
    source $PARAM_FILE
    for v in "${ArrayNumChains[@]}"
    do
        NumChains=$v
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
    done
}

function varyNumWritesPerTx {
    source $PARAM_FILE
    for v in "${ArrayNumWritesPerTx[@]}"
    do
        NumWritesPerTx=$v
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
    done
}

function varyNumReadsPerTx {
    source $PARAM_FILE
    for v in "${ArrayNumReadsPerTx[@]}"
    do
        NumReadsPerTx=$v
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
    done
}

function varyKVSize {
    source $PARAM_FILE
    for v in "${ArrayKVSize[@]}"
    do
        KVSize=$v
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
    done
}

function varyBatchSize {
    source $PARAM_FILE
    for v in "${ArrayBatchSize[@]}"
    do
        BatchSize=$v
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
    done
}

function varyNumTxs {
    source $PARAM_FILE
    for v in "${ArrayNumTxs[@]}"
    do
        NumTotalTx=$v
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
    done
}

function runLargeDataExperiment {
  source $PARAM_FILE
  if [[ $RunLargeDataExperiment = "true" ]]
  then    
    NumKVs=10000000
    NumTotalTx=10000000
        rm -rf $DataDir;upCouchDB;runInsertTxs;runReadWriteTxs
  fi
}

function usage () {
    printf "Usage: ./runbenchmarks.sh -f parameter_file_name"
}

PARAM_FILE=""

while getopts ":f:" opt; do
  case $opt in
    f)
      printf "Parameter file: $OPTARG\n"
      PARAM_FILE=$OPTARG;;
    \?)
      printf "Error: invalid parameter -$OPTARG!"  >> /dev/stderr
      usage
      exit 1;;
  esac
done

if [ ! $PARAM_FILE ]
then
  printf "Error: No Parameter file given!"  >> /dev/stderr
  usage
  exit 1
fi

### Run tests
  runWithDefaultParams
  varyNumParallelTxPerChain
  varyNumChains
  varyNumWritesPerTx
  varyNumReadsPerTx
  varyKVSize
  varyBatchSize
  varyNumTxs
  runLargeDataExperiment
