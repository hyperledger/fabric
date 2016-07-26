#!/bin/bash
source ../common.sh

PKG_PATH="github.com/hyperledger/fabric/core/ledger"
FUNCTION_NAME="BenchmarkLedgerSingleKeyTransaction"
NUM_CPUS=4
CHART_DATA_COLUMN="Number of Bytes"

setupAndCompileTest

Key=key
KVSize=100
BatchSize=100
NumBatches=10000
NumWritesToLedger=2

CHART_COLUMN_VALUE=$KVSize

TEST_PARAMS="-Key=$Key, -KVSize=$KVSize, -BatchSize=$BatchSize, -NumBatches=$NumBatches, -NumWritesToLedger=$NumWritesToLedger"
executeTest
