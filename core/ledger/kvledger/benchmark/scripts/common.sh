#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


set -e

#############################################################################################################################
# This shell script contains common functions that can be used across benchmark tests
# Following is the description of the list of variables that this script uses
#
# OUTPUT_DIR_ROOT - Root dir for tests results
# RAW_OUTPUT_FILE - File name that contains the raw output produced by a test that otherwiese is printed on screen
# RESULTS_FILE - File name that contains the test parameters and the time taken by the test (in csv format)
# PKG_NAME - Name of the golang package for the test
# FUNCTION_NAME - Name of the Benchmark function
# TEST_PARAMS - Parameters for the test
# RESULTANT_DIRS - An optional list of dirs whose size needs to be captured in the results in the RESULTS_FILE
#
# The default values for some of the above variables are set in this script and can be overridden by a test specific
# script. For remaining variables, a test specific script needs to set the appropriate values before calling the
# function 'executeTest' of this script
#
# The result file for a test gets created in a csv format in the folder
# $OUTPUT_DIR_ROOT/<last_element_of>$PKG_NAME<segment>/$FUNCTION_NAME
#############################################################################################################################


OUTPUT_DIR_ROOT=`echo /tmp`
RAW_OUTPUT_FILE="output_LTE.log"
RESULTS_FILE="results.csv"

benchmarkLineRegex="^Benchmark.*[[:blank:]]+[[:digit:]]+[[:blank:]]+([[:digit:]]+).*$"
testParamRegex="-\([^=]*\)=\([^,]*\)"

echo "**Note**: This is a Benchmark test. Please make sure to set an appropriate value for ulimit in your OS."
echo "Recommended value is 10000 for default parameters."
echo "Current ulimit=`ulimit -n`"
TESTS_SETUP_DONE=()

## Execute test and generate data file
function executeTest {
  runTestSetup
  cmd="go test -v -timeout 1000m $PKG_NAME -testParams=\"$TEST_PARAMS\" -bench=$FUNCTION_NAME"
  echo $cmd
  RAW_OUTPUT=`eval $cmd || true`
  if [[ "$RAW_OUTPUT" == *"FAIL"* ]]; then
    printf "%s\n" "$RAW_OUTPUT"
    echo "Failed to run the test.";
    return 1;
  fi
  writeResults
}

function writeResults {
  outputDir=`getOuputDir`
  echo "Test Output Start:"
  echo "$RAW_OUTPUT"
  echo "Test Output Finish"
  while read -r line; do
    echo $line >> $outputDir/$RAW_OUTPUT_FILE
    if [[ $line =~ $benchmarkLineRegex ]]; then
      resultsDataLine="`extractParamValues`, `nanosToSec ${BASH_REMATCH[1]}`"
      for d in $RESULTANT_DIRS; do
        if [ -d $d ]
          then
            dirSizeKBs=`du -sk $d | cut -f1`
            resultsDataLine="$resultsDataLine, `kbsToMbs $dirSizeKBs`"
        else
          resultsDataLine="$resultsDataLine, DoesNotExist"
        fi
        done
        echo $resultsDataLine >> $outputDir/$RESULTS_FILE
    fi
  done <<< "$RAW_OUTPUT"
}

function runTestSetup {
  outputDir=`getOuputDir`
  for d in ${TESTS_SETUP_DONE[@]}
    do
      if [ $d == $outputDir ]
      then
        return
      fi
    done
  createOutputDir
  writeResultsFileHeader
  TESTS_SETUP_DONE+=($outputDir)
}

function writeResultsFileHeader {
  outputDir=`getOuputDir`
  echo "" >> $outputDir/$RESULTS_FILE
  echo "# `date`" >> $outputDir/$RESULTS_FILE
  headerLine="# `extractParamNames`, Time_Spent(s)"
  for d in $RESULTANT_DIRS; do
    headerLine="$headerLine, Size_$(basename $d)(mb)"
  done
  echo "$headerLine" >> $outputDir/$RESULTS_FILE
}

function extractParamNames {
  echo $TEST_PARAMS | sed "s/$testParamRegex/\1/g"
}

function extractParamValues {
  echo $TEST_PARAMS | sed "s/$testParamRegex/\2/g"
}

function getOuputDir {
  pkgName=$(basename $PKG_NAME)
  outputDir="$OUTPUT_DIR_ROOT/$pkgName/$FUNCTION_NAME"
  if [ ! -z "$OUTPUT_DIR" ]; then
    outputDir="$OUTPUT_DIR_ROOT/$pkgName/$OUTPUT_DIR"
  fi
  echo $outputDir
}

function createOutputDir {
  outputDir=`getOuputDir`
  if [ ! -d "$outputDir" ]; then
    mkdir -p $outputDir
  else
    echo "INFO: outputDIR [$outputDir] already exists. Output will be appended to existing file"
  fi
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

function nanosToSec {
  nanos=$1
  echo $(awk "BEGIN {printf \"%.2f\", ${nanos}/1000000000}")
}

function kbsToMbs {
  kbs=$1
  echo $(awk "BEGIN {printf \"%.2f\", ${kbs}/1024}")
}

function upCouchDB {
  if [ "$useCouchDB" == "true" ];
  then
    downCouchDB
    echo "Starting couchdb container on port 5984..."
    export COUCHDB_ADDR=localhost:5984
    docker run --publish 5984:5984 --detach --name couchdb couchdb:2.3 >/dev/null
    sleep 5
  fi
}

function downCouchDB {    
    couch_id=$(docker ps -aq --filter 'ancestor=couchdb:2.3')
    if [ "$couch_id" != "" ]; then
      echo "Stopping couchdb container (id: $couch_id)..."
      docker rm -f $couch_id &>/dev/null
    fi
    sleep 5
}

trap downCouchDB EXIT