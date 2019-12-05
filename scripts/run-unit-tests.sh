#!/bin/bash
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
set -eo pipefail

# regexes for packages to exclude from unit test
excluded_packages=(
  "/integration(/|$)"
)

# packages that are only tested when they (or their deps) change
conditional_packages=(
  "github.com/hyperledger/fabric/gossip"
)

# Retrieves the full directory path of all packages that have been changed
changed=$(git diff --dirstat=files,0 "HEAD^..HEAD" | awk '{$1=$1};1' | cut -d' ' -f2)

for dir in ${conditional_packages[@]}; do
  # Retrieves the entire dependency tree of the conditional package
  deps=($(
    go list -f '{{ .Deps }}' -json "${dir}/..." |
      grep -E "github.com/hyperledger" |
      awk '{$1=$1};1' |
      grep -v "\.\.\." |
      grep -v vendor |
      grep "^\"github" |
      cut -d',' -f1
  ))

  # Add the package itself
  deps+=("${dir}")

  # Checks to see if each of the changed packages is a dependency of the
  # conditional package
  for change in ${changed[@]}; do
    for dep in ${deps[@]}; do
      regex="^${dep}"
      # Checks to see if the dependency is a superset of the change
      if [[ "github.com/hyperledger/fabric/${change}" =~ ${regex} ]]; then
        # If the dependency was a superset of the change we remove
        # the package from the conditional set. Later we will remove
        # all packages that remain in the conditional set
        if [[ ! "${conditional_packages[*]}" =~ ${dir} ]]; then
          delete=("${dir}")
          conditional_packages=(${conditional_packages[@]/$delete})
        fi
      fi
    done
  done
  unset deps
done

test_packages=$(go list ./...)

# Remove conditional packages
for excluded in ${conditional_packages[@]}; do
  test_packages=$(echo ${test_packages} | tr ' ' '\n' | grep -v "${excluded}")
done

# Remove explicitly exclude packages via regex
for excluded in ${excluded_packages[@]}; do
  test_packages=$(echo ${test_packages} | tr ' ' '\n' | grep -Ev ${excluded})
done

gotestsum --junitfile fabric_unit_report.xml --no-summary=skipped -- -cover -race -tags pkcs11 -short -timeout=20m ${test_packages[@]}
