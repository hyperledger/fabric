#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -eo pipefail

# regexes for packages to exclude from unit test
excluded_packages=(
  "/integration(/|$)"
  "fakes"
  "mocks"
  "mock"
  "fake"
)

# packages that are only tested when they (or their deps) change
# specified by its path relative to the root of the repo, i.e. core/container
conditional_packages=(
  "gossip"
)

changed=$(git diff --dirstat=files,0 "HEAD^..HEAD" | awk '{$1=$1};1' | cut -d' ' -f2)
declare -a deps excludes
for dir in ${conditional_packages[@]}; do
  deps+=$(
    go list -f '{{ .Deps }}' -json "github.com/hyperledger/fabric/${dir}/..." |
      grep -E "github.com/hyperledger" |
      awk '{$1=$1};1' |
      grep -v vendor |
      grep "^\"github" |
      cut -d',' -f1 |
      sort |
      uniq
  )
  for change in ${changed[@]}; do
    if [[ "${deps[*]}" =~ ${change} ]]; then
      if [[ ! "${excludes[*]}" =~ ${dir} ]]; then
        excludes+=("${dir}")
      fi
    fi
  done
done

testPackages=$(go list ./...)
for excluded in ${excludes[@]}; do
  testPackages=$(echo ${testPackages} | tr ' ' '\n' | grep -v ${excluded})
done
for excluded in ${excluded_packages[@]}; do
  testPackages=$(echo ${testPackages} | tr ' ' '\n' | grep -Ev ${excluded})
done

if [[ "${ENABLE_JUNIT}" == "true" ]]; then
    gotestsum --junitfile unit_report.xml --no-summary=skipped -- -cover -race -tags pkcs11 -short -timeout=20m ${testPackages}
else
    gotestsum --no-summary=skipped -- -cover -race -tags pkcs11 -short -timeout=20m ${testPackages}
fi
