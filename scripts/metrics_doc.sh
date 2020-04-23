#!/bin/bash -e

# Copyright IBM Corp All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

fabric_dir="$(cd "$(dirname "$0")/.." && pwd)"
metrics_doc="${fabric_dir}/docs/source/metrics_reference.rst"

generate_doc() {
    local gendoc_command="go run github.com/hyperledger/fabric/common/metrics/cmd/gendoc"
    local orderer_prom
    local orderer_statsd
    local peer_prom
    local peer_statsd

    local orderer_deps=()
    while IFS= read -r pkg; do orderer_deps+=("$pkg"); done < <(go list -deps github.com/hyperledger/fabric/cmd/orderer | sort -u | grep hyperledger)
    orderer_prom="$($gendoc_command -template <(echo '{{PrometheusTable}}') "${orderer_deps[@]}")"
    orderer_statsd="$($gendoc_command -template <(echo '{{StatsdTable}}') "${orderer_deps[@]}")"

    local peer_deps=()
    while IFS= read -r pkg; do peer_deps+=("$pkg"); done < <(go list -deps github.com/hyperledger/fabric/cmd/peer | sort -u | grep hyperledger)
    peer_prom="$($gendoc_command -template <(echo '{{PrometheusTable}}') "${peer_deps[@]}")"
    peer_statsd="$($gendoc_command -template <(echo '{{StatsdTable}}') "${peer_deps[@]}")"

cat <<eof
Metrics Reference
=================

Orderer Metrics
---------------

Prometheus
~~~~~~~~~~

The following orderer metrics are exported for consumption by Prometheus.

${orderer_prom}

StatsD
~~~~~~

The following orderer metrics are emitted for consumption by StatsD. The
\`\`%{variable_name}\`\` nomenclature represents segments that vary based on
context.

For example, \`\`%{channel}\`\` will be replaced with the name of the channel
associated with the metric.

${orderer_statsd}

Peer Metrics
------------

Prometheus
~~~~~~~~~~

The following peer metrics are exported for consumption by Prometheus.

${peer_prom}

StatsD
~~~~~~

The following peer metrics are emitted for consumption by StatsD. The
\`\`%{variable_name}\`\` nomenclature represents segments that vary based on
context.

For example, \`\`%{channel}\`\` will be replaced with the name of the channel
associated with the metric.

${peer_statsd}

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
eof
}


case "$1" in
    # check if the metrics documentation is up to date with the metrics
    # options in the tree
    "check")
        if [ -n "$(diff -u <(generate_doc) "${metrics_doc}")" ]; then
            echo "The Fabric metrics reference documentation is out of date."
            echo "Please run '$0 generate' to update the documentation."
            exit 1
        fi
        ;;

    # generate the metrics documentation
    "generate")
         generate_doc > "${metrics_doc}"
        ;;

    *)
        echo "Please specify check or generate"
        exit 1
        ;;
esac
