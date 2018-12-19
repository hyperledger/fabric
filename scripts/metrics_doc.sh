#!/bin/bash -e

# Copyright IBM Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"
metrics_template="${fabric_dir}/docs/source/metrics_reference.rst.tmpl"
metrics_doc="${fabric_dir}/docs/source/metrics_reference.rst"

gendoc_command="go run github.com/hyperledger/fabric/common/metrics/cmd/gendoc -template ${metrics_template}"

case "$1" in
    # check if the metrics documentation is up to date with the metrics
    # options in the tree
    "check")
        if [ -n "$(diff -u <(cd ${fabric_dir} && ${gendoc_command}) ${metrics_doc})" ]; then
            echo "The Fabric metrics reference documentation is out of date."
            echo "Please run '$0 generate' to update the documentation."
            exit 1
        fi
        ;;

    # generate the metrics documentation
    "generate")
         (cd "${fabric_dir}" && ${gendoc_command} > ${metrics_doc})
        ;;

    *)
        echo "Please specify check or generate"
        exit 1
        ;;
esac
