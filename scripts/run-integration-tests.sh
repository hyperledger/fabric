#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

# Use ginkgo to run integration tests. If arguments are provided to the
# script, they are treated as the directories containing the tests to run.
# When no arguments are provided, all integration tests are executed.

set -eu

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd ${fabric_dir}

declare -a test_dirs
test_dirs=($(
 go list -f '{{if or (len .TestGoFiles | ne 0) (len .XTestGoFiles | ne 0)}}{{println .Dir}}{{end}}' ./... | \
 grep integration | \
 sed "s,${fabric_dir},.,g"
))

total_agents=${SYSTEM_TOTALJOBSINPHASE:-1}   # standard VSTS variables available using parallel execution; total number of parallel jobs running
agent_number=${SYSTEM_JOBPOSITIONINPHASE:-1} # current job position

declare -a dirs
test_count=${#test_dirs[@]}
for ((i = "$agent_number"; i <= "$test_count"; )); do
  dirs+=("${test_dirs[$i - 1]}")
  i=$((${i} + ${total_agents}))
done

printf "\nRunning the following test suites:\n\n$(echo ${dirs[*]} | tr -s ' ' '\n')\n\nStarting tests...\n\n"
ginkgo -keepGoing --slowSpecThreshold 60 ${dirs[*]}
