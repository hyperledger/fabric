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
cd "$fabric_dir"

declare -a test_dirs
if [ $# -eq 0 ]
  then
    while IFS='' read -r line; do test_dirs+=("$line"); done < <(
      go list -f '{{ if or (len .TestGoFiles | ne 0) (len .XTestGoFiles | ne 0) }}{{ println .Dir }}{{ end }}' ./... | \
        grep integration | \
        sed s,"${fabric_dir}",.,g
    )
else
  for arg in "$@"; do test_dirs+=("./integration/$arg"); done
fi

total_agents=${SYSTEM_TOTALJOBSINPHASE:-1}   # standard VSTS variables available using parallel execution; total number of parallel jobs running
agent_number=${SYSTEM_JOBPOSITIONINPHASE:-1} # current job position

declare -a dirs
for ((i = "$agent_number"; i <= "${#test_dirs[@]}"; )); do
  dirs+=("${test_dirs[$i - 1]}")
  i=$((i + total_agents))
done

printf "\nRunning the following test suites:\n\n%s\n\nStarting tests...\n\n" "$(echo "${dirs[@]}" | tr -s ' ' '\n')"

ginkgo --keep-going --slow-spec-threshold 60s --timeout 24h "${dirs[@]}"
