#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -e

# regexes for packages to exclude from unit test
excluded_packages=(
    "/integration(/|$)"
)

# regexes for packages that must be run serially
serial_packages=(
    "github.com/hyperledger/fabric/gossip"
)

# packages which need to be tested with build tag pluginsenabled
plugin_packages=(
    "github.com/hyperledger/fabric/core/scc"
)

# packages which need to be tested with build tag pkcs11
pkcs11_packages=(
    "github.com/hyperledger/fabric/bccsp/..."
)

# obtain packages changed since some git refspec
packages_diff() {
    git -C "${GOPATH}/src/github.com/hyperledger/fabric" diff --no-commit-id --name-only -r "${1:-HEAD}" |
        grep '.go$' | grep -Ev '^vendor/|^build/' | \
        sed 's%/[^/]*$%/%' | sort -u | \
        awk '{print "github.com/hyperledger/fabric/"$1"..."}'
}

# "go list" packages and filter out excluded packages
list_and_filter() {
    local filter=$(local IFS='|' ; echo "${excluded_packages[*]}")
    if [ -n "$filter" ]; then
        go list $@ 2>/dev/null | grep -Ev "${filter}" || true
    else
        go list $@ 2>/dev/null
    fi
}

# remove packages that must be tested serially
parallel_test_packages() {
    local filter=$(local IFS='|' ; echo "${serial_packages[*]}")
    if [ -n "$filter" ]; then
        echo "$@" | grep -Ev $(local IFS='|' ; echo "${serial_packages[*]}") || true
    else
        echo "$@"
    fi
}

# get packages that must be tested serially
serial_test_packages() {
    local filter=$(local IFS='|' ; echo "${serial_packages[*]}")
    if [ -n "$filter" ]; then
        echo "$@" | grep -E "${filter}" || true
    else
        echo "$@"
    fi
}

# "go test" the provided packages. Packages that are not prsent in the serial package list
# will be tested in parallel
run_tests() {
    local flags="-cover"
    if [ -n "${VERBOSE}" ]; then
      flags="${flags} -v"
    fi

    echo ${GO_TAGS}

    time {
        local parallel=$(parallel_test_packages "$@")
        if [ -n "${parallel}" ]; then
            go test ${flags} -tags "$GO_TAGS" ${parallel[@]} -short -timeout=20m
        fi

        local serial=$(serial_test_packages "$@")
        if [ -n "${serial}" ]; then
            go test ${flags} -tags "$GO_TAGS" ${serial[@]} -short -p 1 -timeout=20m
        fi
    }
}

# "go test" the provided packages and generate code coverage reports.
run_tests_with_coverage() {
    # run the tests serially
    time go test -p 1 -cover -coverprofile=profile_tmp.cov -tags "$GO_TAGS" $@ -timeout=20m
    tail -n +2 profile_tmp.cov >> profile.cov && rm profile_tmp.cov
}

main() {
    # place the cache directory into the default build tree if it exists
    if [ -d "${GOPATH}/src/github.com/hyperledger/fabric/.build" ]; then
        export GOCACHE="${GOPATH}/src/github.com/hyperledger/fabric/.build/go-cache"
    fi

    # default behavior is to run all tests
    local package_spec=${TEST_PKGS:-github.com/hyperledger/fabric/...}

    # extra exclusions for ppc and s390x
    local arch=`uname -m`
    if [ x${arch} == xppc64le -o x${arch} == xs390x ]; then
        excluded_packages+=("github.com/hyperledger/fabric/core/chaincode/platforms/java")
    fi

    # when running a "verify" job, only test packages that have changed
    if [ "${JOB_TYPE}" = "VERIFY" ]; then
        # first check for uncommitted changes
        package_spec=$(packages_diff HEAD)
        if [ -z "${package_spec}" ]; then
            # next check for changes in the latest commit - typically this will
            # be for CI only, but could also handle a committed change before
            # pushing to Gerrit
            package_spec=$(packages_diff HEAD^)
        fi
    fi

    # expand the package spec into an array of packages
    local -a packages=$(list_and_filter ${package_spec})

    if [ -z "${packages}" ]; then
        echo "Nothing to test!!!"
    elif [ "${JOB_TYPE}" = "PROFILE" ]; then
        echo "mode: set" > profile.cov
        run_tests_with_coverage "${packages[@]}"
        GO_TAGS="${GO_TAGS} pluginsenabled" run_tests_with_coverage "${plugin_packages[@]}"
        GO_TAGS="${GO_TAGS} pkcs11" run_tests_with_coverage "${pkcs11_packages[@]}"
        gocov convert profile.cov | gocov-xml > report.xml
    else
        run_tests "${packages[@]}"
        GO_TAGS="${GO_TAGS} pluginsenabled" run_tests "${plugin_packages[@]}"
        GO_TAGS="${GO_TAGS} pkcs11" run_tests "${pkcs11_packages[@]}"
    fi
}

main
