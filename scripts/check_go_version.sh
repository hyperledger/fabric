#!/bin/bash
#
# SPDX-License-Identifier: Apache-2.0
#
# This is based on check-go-version from coreos/dex by ericchiang

set -e

SUPPORTED_VERSION="go"$(grep "GO_VER" ci.properties |cut -d'=' -f2-|cut -d'.' -f1-2)
MAJOR_VERSION=$(go version|cut -d' ' -f3|cut -d'.' -f1-2)
if [ $MAJOR_VERSION != $SUPPORTED_VERSION ]; then
	>&2 echo "ERROR: ${SUPPORTED_VERSION}.x is required to build Fabric.  Please update your Go installation: https://golang.org/dl/"
	exit 2
fi