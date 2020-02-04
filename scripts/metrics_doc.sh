#!/bin/bash -e

# Copyright IBM Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"
metrics_template="${fabric_dir}/docs/source/metrics_reference.rst.tmpl"
metrics_doc="${fabric_dir}/docs/source/metrics_reference.rst"

orderer_deps=()
while IFS= read -r pkg; do orderer_deps+=("$pkg"); done < <(go list -deps github.com/hyperledger/fabric/cmd/orderer | sort -u | grep hyperledger)

peer_deps=()
while IFS= read -r pkg; do peer_deps+=("$pkg"); done < <(go list -deps github.com/hyperledger/fabric/cmd/peer | sort -u | grep hyperledger/fabric/)

peer="peerpackages"
orderer="ordererpackages"
gendoc_command="go run github.com/hyperledger/fabric/common/metrics/cmd/gendoc -template ${metrics_template} ${orderer} ${orderer_deps[*]} ${peer} ${peer_deps[*]}"


case "$1" in
    # check if the metrics documentation is up to date with the metrics
    # options in the tree
    "check")
        if [ -n "$(diff -u <(cd "${fabric_dir}" && ${gendoc_command}) "${metrics_doc}")" ]; then
            echo "The Fabric metrics reference documentation is out of date."
            echo "Please run '$0 generate' to update the documentation."
            exit 1
        fi
        ;;

    # generate the metrics documentation
    "generate")
         (cd "${fabric_dir}" && ${gendoc_command} > "${metrics_doc}")
        ;;

    *)
        echo "Please specify check or generate"
        exit 1
        ;;
esac
