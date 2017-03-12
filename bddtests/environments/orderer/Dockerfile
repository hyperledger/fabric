FROM hyperledger/fabric-testenv
WORKDIR /opt/gopath/src/github.com/hyperledger/fabric/orderer
RUN apt-get update \
    && apt-get install -y zookeeper jq \
    && rm -rf /var/lib/apt/lists/*
ADD docker-entrypoint.sh /docker-entrypoint.sh
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["orderer"]
