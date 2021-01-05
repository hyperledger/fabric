#!/bin/bash -e

# Copyright IBM Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"
swagger_tags="${fabric_dir}/swagger/tags.json"
swagger_doc="${fabric_dir}/swagger/swagger-fabric.json"

check_spec() {
    swagger_doc_check="${fabric_dir}/swagger/swagger-fabric-check.json"
    swagger generate spec -o "$swagger_doc_check" --scan-models --exclude-deps --input "$swagger_tags"
    if [ -n "$(diff "$swagger_doc_check" "$swagger_doc")" ]; then
        echo "The Fabric swagger is out of date."
        echo "Please run '$0 generate' to update the swagger."
        rm "$swagger_doc_check"
        exit 1
    fi
    rm "$swagger_doc_check"
}

case "$1" in
    # check if the swagger is up to date with the swagger
    # options in the tree
    "check")
        check_spec
    ;;

    # generate the swagger
    "generate")
        swagger generate spec -o "$swagger_doc" --scan-models --exclude-deps --input "$swagger_tags"
    ;;

    *)
        echo "Please specify check or generate"
        exit 1
    ;;
esac

