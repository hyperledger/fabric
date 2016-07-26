#!/bin/bash

set -e

OUTPUT_DIR_ROOT=`echo ~/obc_perf/output`
DB_DIR_ROOT=`echo ~/obc_perf/db`
BINARY_DIR=`echo ~/obc_perf/bin`

mkdir -p $OUTPUT_DIR_ROOT
mkdir -p $DB_DIR_ROOT
mkdir -p $BINARY_DIR

BENCHMARK_OUTPUT_FILE="benchmark.out"
CHART_DATA_FILE="chart.dat"
CHART_FILE="chart.ps"

regex="^Benchmark.*[[:blank:]]+[[:digit:]]+[[:blank:]]+([[:digit:]]+).*$"
function writeBenchmarkOutput {
  #echo "$@"
  outFile=$1
  benchmarkFile=$2
  paramValue=$3
  cmdOutput=$4
  echo "outFile=$outFile, benchmarkFile=$benchmarkFile, paramValue=$paramValue"
  echo "Test Output Start:"
  echo "$cmdOutput"
  echo "Test Output Finish"
  while read -r line; do
    echo $line >> $outFile
    if [[ $line =~ $regex ]]; then
      benchmarkDataLine="$paramValue  ${BASH_REMATCH[1]}"
      echo $benchmarkDataLine >> $benchmarkFile
    fi
  done <<< "$cmdOutput"
}

function setupAndCompileTest {
  createOutputDir
  configureDBPath
  compileTest
  writeBenchmarkHeader
}

function compileTest {
  cmd="go test $PKG_PATH -c -o `getBinaryFileName`"
  `eval $cmd`
}

function writeBenchmarkHeader {
  outputDir=`getOuputDir`
  echo "# `date`" >> $outputDir/$CHART_DATA_FILE
  echo "# TEST_PARAMS $TEST_PARAMS" >> $outputDir/$CHART_DATA_FILE
  echo "# $CHART_DATA_COLUMN | ns/ops" >> $outputDir/$CHART_DATA_FILE
}

## Execute test and generate data file
function executeTest {
  cmd="`getBinaryFileName` -testParams=\"$TEST_PARAMS\" -test.run=XXX -test.bench=$FUNCTION_NAME -test.cpu=$NUM_CPUS $ADDITIONAL_TEST_FLAGS $PKG_PATH"
  outputDir=`getOuputDir`
  dbDir=`getDBDir`
  echo ""
  echo "Executing test... [OUTPUT_DIR=$outputDir, DB_DIR=$dbDir]"
  echo $cmd
  cmdOutput=`eval $cmd`
  writeBenchmarkOutput $outputDir/$BENCHMARK_OUTPUT_FILE $outputDir/$CHART_DATA_FILE $CHART_COLUMN_VALUE "$cmdOutput"
}

function getBinaryFileName {
  pkgName=$(basename $PKG_PATH)
  echo "$BINARY_DIR/$pkgName.test"
}

function getOuputDir {
  pkgName=$(basename $PKG_PATH)
  outputDir="$OUTPUT_DIR_ROOT/$pkgName/$FUNCTION_NAME"
  if [ ! -z "$OUTPUT_DIR" ]; then
    outputDir="$OUTPUT_DIR_ROOT/$pkgName/$OUTPUT_DIR"
  fi
  echo $outputDir
}

function getDBDir {
  pkgName=$(basename $PKG_PATH)
  dbDir="$DB_DIR_ROOT/$pkgName/$FUNCTION_NAME"
  if [ ! -z "$DB_DIR" ]; then
    dbDir="$DB_DIR_ROOT/$pkgName/$DB_DIR"
  fi
  echo $dbDir
}

function createOutputDir {
  outputDir=`getOuputDir`
  if [ ! -d "$outputDir" ]; then
    mkdir -p $outputDir
  else
    echo "INFO: outputDIR [$outputDir] already exists. Output will be appended to existing file"
  fi
}

function configureDBPath {
  dbDir=`getDBDir`
  if [ -d "$dbDir" ]; then
    echo "INFO: dbDir [$dbDir] already exists. Data will be merged in the existing data"
  fi
  ulimit -n 10000
  echo "setting ulimit=`ulimit -n`"
  export PEER_FILESYSTEMPATH="$dbDir"
}

function constructChart {
  outputDir=`getOuputDir`
  gnuplot -e "dataFile='$outputDir/$CHART_DATA_FILE'" plot.pg > $outputDir/$CHART_FILE
}

function openChart {
  outputDir=`getOuputDir`
  open "$outputDir/$CHART_FILE"
}

function clearOSCache {
  platform=`uname`
  if [[ $platform == 'Darwin' ]]; then
    echo "Clearing os cache"
    sudo purge
  else
    echo "WARNING: Platform [$platform] is not supported for clearing os cache."
  fi
}
