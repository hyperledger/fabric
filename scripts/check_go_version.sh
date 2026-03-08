#!/bin/bash
#
# SPDX-License-Identifier: Apache-2.0

set -e

CI_VERSION=$1
GO_VERSION="$(go version | cut -f3 -d' ' | sed -E 's/^go//')"

fail() {
    >&2 echo "ERROR: ${CI_VERSION} is required to build Fabric and you are using ${GO_VERSION}. Please update go."
    exit 2
}

vpos () {
    local v
    v="$(echo "$1" | sed -E 's/([[:digit:]]+)\.([[:digit:]]+)(\.([[:digit:]]+))?/'"\\$2"'/g')"
    echo "${v:-0}"
}
version() {
    vpos "$1" 1
}
release() {
    vpos "$1" 2
}
patch () {
    vpos "$1" 4
}

# major versions must match
[ "$(version "$GO_VERSION")" -eq "$(version "$CI_VERSION")" ] || fail

# go release must be >= ci release
[ "$(release "$GO_VERSION")" -ge "$(release "$CI_VERSION")" ] || fail

# If go release is greater than the expected ci release, things may not work, print a warning.
if [ "$(release "$GO_VERSION")" -gt "$(release "$CI_VERSION")" ]; then
  echo "WARNING: Your Go release $GO_VERSION is greater than expected release $CI_VERSION, CI may or may not work depending on Go changes."
fi

# if releases are equal, patch must be >= ci patch
if [ "$(release "$GO_VERSION")" -eq "$(release "$CI_VERSION")" ]; then
    [ "$(patch "$GO_VERSION")" -ge "$(patch "$CI_VERSION")" ] || fail
fi

# versions are equal and go release > required ci release
exit 0
