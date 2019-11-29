#!/bin/bash -eu
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

# ----------------------------------------------------------------
# set custom limits
# ----------------------------------------------------------------

cat <<EOF >/etc/security/limits.d/99-hyperledger.conf
# custom limits for hyperledger development

*       soft    nofile          10000
*       hard    nofile          16384
EOF
