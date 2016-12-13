FROM  hyperledger/fabric-testenv
WORKDIR /opt/gopath/src/github.com/hyperledger/fabric/orderer
RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y zookeeper jq \
    && rm -rf /var/lib/apt/lists/*
ENV ORDERER_GENERAL_LEDGERTYPE=ram \
    ORDERER_GENERAL_BATCHTIMEOUT=10s \
    ORDERER_GENERAL_BATCHSIZE=10 \
    ORDERER_GENERAL_MAXWINDOWSIZE=1000 \
    ORDERER_GENERAL_LISTENADDRESS=0.0.0.0 \
    ORDERER_GENERAL_LISTENPORT=7050 \
    ORDERER_RAMLEDGER_HISTORY_SIZE=100 \
    ORDERER_GENERAL_GENESISMETHOD=provisional
ENV ORDERER_GENERAL_ORDERERTYPE=kafka
ADD docker-entrypoint.sh /docker-entrypoint.sh
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["orderer"]
