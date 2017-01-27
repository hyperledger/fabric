#!/usr/bin/env bash

# This script will either start the kafka server, or run the user
# specified command.

# Exit immediately if a pipeline returns a non-zero status.
set -e

ORDERER_EXE=orderer

# handle starting the orderer with an option
if [ "${1:0:1}" = '-' ]; then
  set -- ${ORDERER_EXE} "$@"
fi

# handle default (i.e. no custom options or commands)
if [ "$1" = "${ORDERER_EXE}" ]; then

  case "$ORDERER_GENESIS_ORDERERTYPE" in
  solo)
    ;;
  kafka)
    # make sure at least one broker has started.
    # get the broker list from zookeeper
    if [ -z "$ORDERER_KAFKA_BROKERS" ] ; then
      if [ -z "$ZOOKEEPER_CONNECT" ] ; then
        export ZOOKEEPER_CONNECT="zookeeper:2181"
      fi
      ZK_CLI_EXE="/usr/share/zookeeper/bin/zkCli.sh -server ${ZOOKEEPER_CONNECT}"
      until [ -n "$($ZK_CLI_EXE ls /brokers/ids | grep '^\[')" ] ; do
        echo "No Kafka brokers registered in ZooKeeper. Will try again in 1 second."
        sleep 1
      done
      ORDERER_KAFKA_BROKERS="["
      ORDERER_KAFKA_BROKERS_SEP=""
      for BROKER_ID in $($ZK_CLI_EXE ls /brokers/ids | grep '^\[' | sed 's/[][,]/ /g'); do
        ORDERER_KAFKA_BROKERS=${ORDERER_KAFKA_BROKERS}${ORDERER_KAFKA_BROKERS_SEP}$($ZK_CLI_EXE get /brokers/ids/$BROKER_ID 2>&1 | grep '^{' | jq -j '. | .host,":",.port')
        ORDERER_KAFKA_BROKERS_SEP=","
      done
      export ORDERER_KAFKA_BROKERS="${ORDERER_KAFKA_BROKERS}]"
    fi
    ;;
  sbft)
    ;;
  esac

fi

exec "$@"
