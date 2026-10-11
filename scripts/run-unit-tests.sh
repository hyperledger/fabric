#!/bin/bash

# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

set -eo pipefail

base_dir="$(cd "$(dirname "$0")/.." && pwd)"

# regexes for packages to exclude from unit test
excluded_packages=(
    "/integration(/|$)"
)

no_coverage_packages=(
    "github.com/hyperledger/fabric/core/handlers/library"
)

# packages which need to be tested with build tag pkcs11
pkcs11_packages=(
    "github.com/hyperledger/fabric/internal/peer/common"
)

# join array elements by the specified string
join_by() {
    local IFS="$1"; shift
    [ "$#" -eq 0 ] && return 0
    echo "$*"
}

contains_element() {
    local key="$1"; shift

    for e in "$@"; do [ "$e" == "$key" ] && return 0; done
    return 1
}

# create a grep regex from the provide package spec
package_filter() {
    local -a filter
    if [ "${#@}" -ne 0 ]; then
        while IFS= read -r pkg; do [ -n "$pkg" ] && filter+=("$pkg"); done < <(go list -f '^{{ .ImportPath }}$' "${@}")
    fi

    join_by '|' "${filter[@]}"
}

# obtain packages changed since some git refspec
packages_diff() {
    git -C "${base_dir}" diff --no-commit-id --name-only -r "${1:-HEAD}" |
        (grep '.go$' || true) | \
        sed 's%/[^/]*$%%' | sort -u | \
        awk '{print "github.com/hyperledger/fabric/"$1}'
}

# obtain list of changed packages for verification
changed_packages() {
    local -a changed

    # first check for uncommitted changes
    while IFS= read -r pkg; do changed+=("$pkg"); done < <(packages_diff HEAD)
    if [ "${#changed[@]}" -eq 0 ]; then
        # next check for changes in the latest commit
        while IFS= read -r pkg; do changed+=("$pkg"); done < <(packages_diff HEAD^)
    fi

    join_by $'\n' "${changed[@]}"
}

# "go list" packages and filter out excluded packages
list_and_filter() {
    local filter
    filter=$(join_by '|' "${excluded_packages[@]}")
    if [ -n "$filter" ]; then
        go list "$@" 2>/dev/null | grep -Ev "${filter}" || true
    else
        go list "$@" 2>/dev/null
    fi
}

no_coverage_test_packages() {
    local filter
    filter=$(package_filter "${no_coverage_packages[@]}")
    if [ -n "$filter" ]; then
        join_by $'\n' "$@" | grep -E "$filter" || true
    fi
}

# "go test" the provided packages in parallel
run_tests() {
    local -a flags
    if [ -n "${VERBOSE}" ]; then
        flags+=("-v")
    fi

    local -a race_flags=()
    if [ "$(uname -m)" == "x86_64" ] || [ "$(uname -m)" == "arm64" ]; then
        export GORACE=atexit_sleep_ms=0 # reduce overhead of race
        race_flags+=("-race")
    fi

    GO_TAGS=${GO_TAGS## }
    [ -n "$GO_TAGS" ] && echo "Testing with $GO_TAGS..."

    time {
        if [ "${#@}" -ne 0 ]; then
            go test -cover "${flags[@]}" "${race_flags[@]}" -tags "$GO_TAGS" "$@" -short -timeout=20m -skip=NoCover
        fi

        # The -cover flag changes the import table of the test and the plugin
        # so that they cannot interact with each other.
        # In the name of tests that work with plugins we added the suffix NoCover
        # and do not run these tests with the -cover flag.
        # We run such tests separately, without the -cover flag.
        local -a no_coverage
        while IFS= read -r pkg; do no_coverage+=("$pkg"); done < <(no_coverage_test_packages "$@")
        if [ "${#no_coverage[@]}" -ne 0 ]; then
            echo "test with no coverage"
            go test "${flags[@]}" "${race_flags[@]}" -tags "$GO_TAGS" "${no_coverage[@]}" -short -timeout=20m -run=NoCover
        fi
    }
}

# "go test" the provided packages and generate code coverage reports.
run_tests_with_coverage() {
    # run the tests serially
    time go test -p 1 -cover -coverprofile=profile_tmp.cov -tags "$GO_TAGS" "$@" -timeout=20m -skip=NoCover
    tail -n +2 profile_tmp.cov >> profile.cov && rm profile_tmp.cov
}

main() {
    # default behavior is to run all tests
    local -a package_spec=("${TEST_PKGS:-github.com/hyperledger/fabric/...}")

    # when running a "verify" job, only test packages that have changed
    if [ "${JOB_TYPE}" = "VERIFY" ]; then
        package_spec=()
        while IFS= read -r pkg; do package_spec+=("$pkg"); done < <(changed_packages)
    fi

    # expand the package specs into arrays of packages
    local -a packages packages_with_pkcs11
    while IFS= read -r pkg; do packages+=("$pkg"); done < <(list_and_filter "${package_spec[@]}")
    while IFS= read -r pkg; do contains_element "$pkg" "${packages[@]}" && packages_with_pkcs11+=("$pkg"); done < <(list_and_filter "${pkcs11_packages[@]}")

    local all_packages=( "${packages[@]}" "${packages_with_pkcs11[@]}" "${packages_with_pkcs11[@]}" )
    if [ "${#all_packages[@]}" -eq 0 ]; then
        echo "Nothing to test!!!"
    elif [ "${JOB_TYPE}" = "PROFILE" ]; then
        echo "mode: set" > profile.cov
        [ "${#packages}" -eq 0 ] || run_tests_with_coverage "${packages[@]}"
        [ "${#packages_with_pkcs11}" -eq 0 ] || GO_TAGS="${GO_TAGS} pkcs11" run_tests_with_coverage "${packages_with_pkcs11[@]}"
        gocov convert profile.cov | gocov-xml > report.xml
    else
        [ "${#packages}" -eq 0 ] || run_tests "${packages[@]}"
        [ "${#packages_with_pkcs11}" -eq 0 ] || GO_TAGS="${GO_TAGS} pkcs11" run_tests "${packages_with_pkcs11[@]}"
    fi
}

main
