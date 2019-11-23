#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

# Use ginkgo to run integration tests. If arguments are provided to the
# script, they are treated as the directories containing the tests to run.
# When no arguments are provided, all integration tests are executed.

set -e -u

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"

cd "$fabric_dir"

dirs=()
if [ "${#}" -eq 0 ]; then
  specs=()
  specs=("$(grep -Ril --exclude-dir=vendor --exclude-dir=scripts "RunSpecs" . | grep integration)")
  for spec in ${specs[*]}; do
    dirs+=("$(dirname "${spec}")")
  done
else
  dirs=("$@")
fi

totalAgents=${SYSTEM_TOTALJOBSINPHASE:-0}   # standard VSTS variables available using parallel execution; total number of parallel jobs running
agentNumber=${SYSTEM_JOBPOSITIONINPHASE:-0} # current job position
testCount=${#dirs[@]}

# below conditions are used if parallel pipeline is not used. i.e. pipeline is running with single agent (no parallel configuration)
if [ "$totalAgents" -eq 0 ]; then totalAgents=1; fi
if [ "$agentNumber" -eq 0 ]; then agentNumber=1; fi

declare -a files
for ((i = "$agentNumber"; i <= "$testCount"; )); do
  files+=("${dirs[$i - 1]}")
  i=$((${i} + ${totalAgents}))
done

echo "Running the following test suites: ${files[*]}"
ginkgo -keepGoing --slowSpecThreshold 60 ${files[*]}
