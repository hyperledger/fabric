#!/usr/bin/env bash

# This script will either start the Kafka server, or run the user
# specified command.

# exit immediately if a pipeline returns a non-zero status
set -e

ORDERER_EXE=orderer
KAFKA_GENESIS_PROFILES=(SampleInsecureKafka)

# handle starting the orderer with an option
if [ "${1:0:1}" = '-' ]; then
  set -- ${ORDERER_EXE} "$@"
fi

# check if the Kafka-based orderer is invoked
if [ "$1" = "${ORDERER_EXE}" ]; then
  for PROFILE in "${KAFKA_GENESIS_PROFILES[@]}"; do
    if [ "$ORDERER_GENERAL_GENESISPROFILE" == "$PROFILE" ] ; then
      # make sure at least one broker has started
      # get the broker list from ZooKeeper
      if [ -z "$CONFIGTX_ORDERER_KAFKA_BROKERS" ] ; then
        if [ -z "$ZOOKEEPER_CONNECT" ] ; then
          export ZOOKEEPER_CONNECT="zookeeper:2181"
        fi
        ZK_CLI_EXE="/usr/share/zookeeper/bin/zkCli.sh -server ${ZOOKEEPER_CONNECT}"
        until [ -n "$($ZK_CLI_EXE ls /brokers/ids | grep '^\[')" ] ; do
          echo "No Kafka brokers registered in ZooKeeper. Will try again in 1 second."
          sleep 1
        done
        CONFIGTX_ORDERER_KAFKA_BROKERS="["
        CONFIGTX_ORDERER_KAFKA_BROKERS_SEP=""
        for BROKER_ID in $($ZK_CLI_EXE ls /brokers/ids | grep '^\[' | sed 's/[][,]/ /g'); do
          CONFIGTX_ORDERER_KAFKA_BROKERS=${CONFIGTX_ORDERER_KAFKA_BROKERS}${ORDERER_KAFKA_BROKERS_SEP}$($ZK_CLI_EXE get /brokers/ids/$BROKER_ID 2>&1 | grep '^{' | jq -j '. | .host,":",.port')
          CONFIGTX_ORDERER_KAFKA_BROKERS_SEP=","
        done
        export CONFIGTX_ORDERER_KAFKA_BROKERS="${CONFIGTX_ORDERER_KAFKA_BROKERS}]"
      fi
    fi
  done
fi

exec "$@"
