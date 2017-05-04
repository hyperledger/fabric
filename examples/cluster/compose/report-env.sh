#!/bin/bash

NODES=$1

getip() {
    HOST=$1

    docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $HOST
}

generate_hosts() {
    for NODE in $NODES; do
        echo "$(getip $NODE) $NODE"
    done
}

echo "========================================================================"
echo "Cluster ready!"
echo "========================================================================"
echo
generate_hosts | sort
