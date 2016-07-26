FROM hyperledger/fabric-src:latest
RUN mkdir -p /var/hyperledger/db
COPY bin/* $GOPATH/bin/
WORKDIR $GOPATH/src/github.com/hyperledger/fabric
