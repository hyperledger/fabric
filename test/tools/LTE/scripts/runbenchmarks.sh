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

function varyNumParallelTxPerChain {
    for v in "${ArrayNumParallelTxPerChain[@]}"
    do
        NumParallelTxPerChain=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumChains {
    for v in "${ArrayNumChains[@]}"
    do
        NumChains=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumKeysInEachTx {
    for v in "${ArrayNumKeysInEachTx[@]}"
    do
        NumKeysInEachTx=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyKVSize {
    for v in "${ArrayKVSize[@]}"
    do
        KVSize=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyBatchSize {
    for v in "${ArrayBatchSize[@]}"
    do
        BatchSize=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumParallelTxWithSingleChain {
    NumChains=1
    for v in "${ArrayNumParallelTxWithSingleChain[@]}"
    do
        NumParallelTxPerChain=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumChainsWithNoParallelism {
    NumParallelTxPerChain=1
    for v in "${ArrayNumChainsWithNoParallelism[@]}"
    do
        NumChains=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function varyNumTxs {
    for v in "${ArrayNumTxs[@]}"
    do
        NumTotalTx=$v
        rm -rf $DataDir;runInsertTxs;runReadWriteTxs
    done
}

function runLargeDataExperiment {
    NumKVs=10000000
    NumTotalTx=10000000
    rm -rf $DataDir;runInsertTxs;runReadWriteTxs
}

function upCouchDB {
    downCouchDB
    echo "Starting couchdb container..."
    echo "Please note that host port 5984 will be made reachable "
    docker run --publish 5984:5984 --detach --name couchdb hyperledger/fabric-couchdb >/dev/null
    sleep 2
}

function downCouchDB {
    couch_id=$(docker ps -aq --filter 'ancestor=hyperledger/fabric-couchdb')
    if [ "$couch_id" != "" ]; then
      echo "Stopping couchdb container (id: $couch_id)..."
      docker rm -f $couch_id &>/dev/null
    fi
    sleep 2
}

function cleanup {
  if [ "$useCouchDB" == "yes" ];
  then
    downCouchDB
  fi
}

function usage () {
    printf "Usage: ./runbenchmarks.sh [-f parameter_file_name] [test_name]\nAvailable tests (use \"all\" to run all tests):
varyNumParallelTxPerChain
varyNumChains
varyNumParallelTxWithSingleChain
varyNumChainsWithNoParallelism
varyNumKeysInEachTx
varyKVSize
varyBatchSize
varyNumTxs
runLargeDataExperiment\n"
}

trap cleanup EXIT
PARAM_FILE=""

while getopts ":f:" opt; do
  case $opt in
    f)
      printf "Parameter file: $OPTARG"
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
else
  source $PARAM_FILE
  if [ "$useCouchDB" == "yes" ];
  then
    upCouchDB
  fi
fi
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
    usage
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
    usage
    exit 1 ;;
esac
