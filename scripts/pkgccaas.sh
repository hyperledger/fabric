#!/bin/bash

#
# SPDX-License-Identifier: Apache-2.0
#

function usage() {
    echo "Usage: pkgccaas.sh -l <label> [-m <META-INF directory>] <connection.json file>"
    echo
    echo "  Creates a chaincode package containing a Chaincode As A Server connection.json file"
    echo
    echo "    Flags:"
    echo "    -l <label> - chaincode label"
    echo "    -m <META-INF directory> - state database index definitions for CouchDB"
    echo "    -h - Print this message"
}

function error_exit {
    echo "${1:-"Unknown Error"}" 1>&2
    exit 1
}

while getopts "hl:m:" opt; do
    case "$opt" in
        h)
            usage
            exit 0
            ;;
        l)
            label=${OPTARG}
            ;;
        m)
            metainf=${OPTARG}
            ;;
        *)
            usage
            exit 1
            ;;
    esac
done
shift $((OPTIND-1))

file=$1

if [ -z "$label" ] || [ -z "$file" ]; then
    usage
    exit 1
fi

filename=$(basename "$file")
if [ ! "$filename" = "connection.json" ]; then
    error_exit "Invalid chaincode file $file: ccaas chaincode requires a connection.json file"
fi

if [ ! -d "$file" ] && [ ! -f "$file" ]; then
    error_exit "Cannot find file $file"
fi

if [ -n "$metainf" ]; then
    metadir=$(basename "$metainf")
    if [ "META-INF" != "$metadir" ]; then
        error_exit "Invalid chaincode META-INF directory $metainf: directory name must be 'META-INF'"
    elif [ ! -d "$metainf" ]; then
        error_exit "Cannot find directory $metainf"
    fi
fi

prefix=$(basename "$0")
tempdir=$(mktemp -d -t "$prefix.XXXXXXXX") || error_exit "Error creating temporary directory"

if [ -n "$DEBUG" ]; then
    echo "label = $label"
    echo "file = $file"
    echo "tempdir = $tempdir"
    echo "metainf = $metainf"
fi

mkdir -p "$tempdir/src"
if [ -d "$file" ]; then
    cp -a "$file/"* "$tempdir/src/"
elif [ -f "$file" ]; then
    cp -a "$file" "$tempdir/src/"
fi

if [ -n "$metainf" ]; then
    cp -a "$metainf" "$tempdir/src/"
fi

mkdir -p "$tempdir/pkg"
cat << METADATAJSON-EOF > "$tempdir/pkg/metadata.json"
{
    "type": "ccaas",
    "label": "$label"
}
METADATAJSON-EOF

tar -C "$tempdir/src" -czf "$tempdir/pkg/code.tar.gz" .

tar -C "$tempdir/pkg" -czf "$label.tgz" metadata.json code.tar.gz

rm -Rf "$tempdir"
