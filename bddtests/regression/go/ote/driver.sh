#!/bin/bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#


InvalidArgs=0

#init var
nBroker=0
nPeer=1

while getopts ":l:d:w:x:b:c:t:a:o:k:p:" opt; do
  case $opt in
    # peer environment options
    l)
      CORE_LOGGING_LEVEL=$OPTARG
      export CORE_LOGGING_LEVEL=$CORE_LOGGING_LEVEL
      echo "CORE_LOGGING_LEVEL: $CORE_LOGGING_LEVEL"
      ;;
    d)
      db=$OPTARG
      echo "ledger state database type: $db"
      ;;
    w)
      CORE_SECURITY_LEVEL=$OPTARG
      export CORE_SECURITY_LEVEL=$CORE_SECURITY_LEVEL
      echo "CORE_SECURITY_LEVEL: $CORE_SECURITY_LEVEL"
      ;;
    x)
      CORE_SECURITY_HASHALGORITHM=$OPTARG
      export CORE_SECURITY_HASHALGORITHM=$CORE_SECURITY_HASHALGORITHM
      echo "CORE_SECURITY_HASHALGORITHM: $CORE_SECURITY_HASHALGORITHM"
      ;;

    # orderer environment options
    b)
      CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT=$OPTARG
      export CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT=$CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT
      echo "CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT: $CONFIGTX_ORDERER_BATCHSIZE_MAXMESSAGECOUNT"
      ;;
    c)
      CONFIGTX_ORDERER_BATCHTIMEOUT=$OPTARG
      export CONFIGTX_ORDERER_BATCHTIMEOUT=$CONFIGTX_ORDERER_BATCHTIMEOUT
      echo "CONFIGTX_ORDERER_BATCHTIMEOUT: $CONFIGTX_ORDERER_BATCHTIMEOUT"
      ;;
    t)
      CONFIGTX_ORDERER_ORDERERTYPE=$OPTARG
      export CONFIGTX_ORDERER_ORDERERTYPE=$CONFIGTX_ORDERER_ORDERERTYPE
      echo "CONFIGTX_ORDERER_ORDERERTYPE: $CONFIGTX_ORDERER_ORDERERTYPE"
      if [$nBroker == 0 ] && [ $CONFIGTX_ORDERER_ORDERERTYPE == 'kafka' ]; then
          nBroker=1   # must have at least 1
      fi
      ;;

    # network options
    a)
      Req=$OPTARG
      echo "action: $Req"
      ;;
    k)
      nBroker=$OPTARG
      echo "# of Broker: $nBroker"
      ;;
    p)
      nPeer=$OPTARG
      echo "# of peer: $nPeer"
      ;;
    o)
      nOrderer=$OPTARG
      echo "# of orderer: $nOrderer"
      ;;

    # else
    \?)
      echo "Invalid option: -$OPTARG" >&2
      InvalidArgs=1
      ;;
    :)
      echo "Option -$OPTARG requires an argument." >&2
      InvalidArgs=1
      ;;
  esac
done


if [ $InvalidArgs == 1 ]; then
   echo "Usage: "
   echo " ./driver.sh [opt] [value] "
   echo "    network variables"
   echo "    -a: action [create|add] "
   echo "    -p: number of peers "
   echo "    -o: number of orderers "
   echo "    -k: number of brokers "
   echo " "
   echo "    peer environment variables"
   echo "    -l: core logging level [(deafult = not set)|CRITICAL|ERROR|WARNING|NOTICE|INFO|DEBUG]"
   echo "    -w: core security level [256|384]"
   echo "    -x: core security hash algorithm [SHA2|SHA3]"
   echo "    -d: core ledger state DB [goleveldb|couchdb] "
   echo " "
   echo "    orderer environment variables"
   echo "    -b: batch size [10|msgs in batch/block]"
   echo "    -t: orderer type [solo|kafka] "
   echo "    -c: batch timeout [10s|max secs before send an unfilled batch] "
   echo " "
   exit
fi

if [ $nBroker -gt 0 ] && [ $CONFIGTX_ORDERER_ORDERERTYPE == 'solo' ]; then
    echo "reset Kafka Broker number to 0 due to the CONFIGTX_ORDERER_ORDERERTYPE=$CONFIGTX_ORDERER_ORDERERTYPE"
    nBroker=0
fi

#OS
##OSName=`uname`
##echo "Operating System: $OSName"


dbType=`echo "$db" | awk '{print tolower($0)}'`
echo "action=$Req nPeer=$nPeer nBroker=$nBroker nOrderer=$nOrderer dbType=$dbType"
VP=`docker ps -a | grep 'peer node start' | wc -l`
echo "existing peers: $VP"


echo "remove old docker-composer.yml"
rm -f docker-compose.yml

# form json input file
if [ $nBroker == 0 ]; then
    #jsonFILE="network_solo.json"
    jsonFILE="network.json"
else
#    jsonFILE="network_kafka.json"
    jsonFILE="network.json"
fi
echo "jsonFILE $jsonFILE"

# create docker compose yml
if [ $Req == "add" ]; then
    N1=$[nPeer+VP]
    N=$[N1]
    VPN="peer"$[N-1]
else
    N1=$nPeer
    N=$[N1 - 1]
    VPN="peer"$N
fi

## echo "N1=$N1 VP=$VP nPeer=$nPeer VPN=$VPN"

node json2yml.js $jsonFILE $N1 $nOrderer $nBroker $dbType

## sed 's/-x86_64/TEST/g' docker-compose.yml > ss.yml
## cp ss.yml docker-compose.yml
# create network
if [ $Req == "create" ]; then

   #CHANGED FROM ORIG SCRIPT
   #docker-compose -f docker-compose.yml up -d --force-recreate $VPN cli
   docker-compose -f docker-compose.yml up -d --force-recreate $VPN
   for ((i=1; i<$nOrderer; i++))
   do
       tmpOrd="orderer"$i
       docker-compose -f docker-compose.yml up -d $tmpOrd
   done
fi

if [ $Req == "add" ]; then
   docker-compose -f docker-compose.yml up -d $VPN

fi

exit
