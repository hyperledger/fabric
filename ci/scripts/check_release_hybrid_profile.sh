#!/usr/bin/env bash
# Copyright The Fabric-PQ contributors. All rights reserved.
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if grep -Eq 'developmentOnly:[[:space:]]*true' pq-crypto/hybrid_profile.go; then
    echo "release blocked: hybrid profile registry contains a development-only profile" >&2
    exit 1
fi
