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

# packages that must be run serially
serial_packages=(
    "github.com/hyperledger/fabric/gossip/..."
)

# packages which need to be tested with build tag pkcs11
pkcs11_packages=(
    "github.com/hyperledger/fabric/bccsp/factory"
    "github.com/hyperledger/fabric/bccsp/pkcs11"
    "github.com/hyperledger/fabric/internal/peer/common"
)

# packages that are only tested when they (or their deps) change
conditional_packages=(
    "github.com/hyperledger/fabric/gossip/..."
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
    local excluded conditional filter

    excluded=("${excluded_packages[@]}")
    conditional=$(package_filter "${conditional_packages[@]}")
    if [ -n "$conditional" ]; then
        excluded+=("$conditional")
    fi

    filter=$(join_by '|' "${excluded[@]}")
    if [ -n "$filter" ]; then
        go list "$@" 2>/dev/null | grep -Ev "${filter}" || true
    else
        go list "$@" 2>/dev/null
    fi
}

# list conditional packages that have been changed
list_changed_conditional() {
    [ "${#conditional_packages[@]}" -eq 0 ] && return 0

    local changed
    changed=$(changed_packages)

    local -a additional_packages
    for pkg in $(go list "${conditional_packages[@]}"); do
        local dep_regexp
        dep_regexp=$(go list -f '{{ join .Deps "$|" }}' "$pkg")
        echo "${changed}" | grep -qE "$dep_regexp" && additional_packages+=("$pkg")
        echo "${changed}" | grep -qE "$pkg\$" && additional_packages+=("$pkg")
    done

    join_by $'\n' "${additional_packages[@]}"
}

# remove packages that must be tested serially
parallel_test_packages() {
    local filter
    filter=$(package_filter "${serial_packages[@]}")
    if [ -n "$filter" ]; then
        join_by $'\n' "$@" | grep -Ev "$filter" || true
    else
        join_by $'\n' "$@"
    fi
}

# get packages that must be tested serially
serial_test_packages() {
    local filter
    filter=$(package_filter "${serial_packages[@]}")
    if [ -n "$filter" ]; then
        join_by $'\n' "$@" | grep -E "$filter" || true
    fi
}

# "go test" the provided packages. Packages that are not present in the serial package list
# will be tested in parallel
run_tests() {
    local -a flags=("-cover")
    if [ -n "${VERBOSE}" ]; then
        flags+=("-v")
    fi

    local -a race_flags=()
    if [ "$(uname -m)" == "x86_64" ]; then
        export GORACE=atexit_sleep_ms=0 # reduce overhead of race
        race_flags+=("-race")
    fi

    GO_TAGS=${GO_TAGS## }
    [ -n "$GO_TAGS" ] && echo "Testing with $GO_TAGS..."

    time {
        local -a serial
        while IFS= read -r pkg; do serial+=("$pkg"); done < <(serial_test_packages "$@")
        if [ "${#serial[@]}" -ne 0 ]; then
            go test "${flags[@]}" -failfast -tags "$GO_TAGS" "${serial[@]}" -short -p 1 -timeout=20m
        fi

        local -a parallel
        while IFS= read -r pkg; do parallel+=("$pkg"); done < <(parallel_test_packages "$@")
        if [ "${#parallel[@]}" -ne 0 ]; then
            go test "${flags[@]}" "${race_flags[@]}" -tags "$GO_TAGS" "${parallel[@]}" -short -timeout=20m
        fi
    }
}

# "go test" the provided packages and generate code coverage reports.
run_tests_with_coverage() {
    # run the tests serially
    time go test -p 1 -cover -coverprofile=profile_tmp.cov -tags "$GO_TAGS" "$@" -timeout=20m
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

    # run everything when profiling
    if [ "${JOB_TYPE}" = "PROFILE" ]; then
        conditional_packages=()
    fi

    # expand the package specs into arrays of packages
    local -a candidates packages packages_with_pkcs11
    while IFS= read -r pkg; do candidates+=("$pkg"); done < <(go list "${package_spec[@]}")
    while IFS= read -r pkg; do packages+=("$pkg"); done < <(list_and_filter "${package_spec[@]}")
    while IFS= read -r pkg; do contains_element "$pkg" "${candidates[@]}" && packages+=("$pkg"); done < <(list_changed_conditional)
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
