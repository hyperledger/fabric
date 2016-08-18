FROM hyperledger/fabric-src:_TAG_
RUN mkdir -p /var/hyperledger/db
COPY bin/* $GOPATH/bin/
WORKDIR $GOPATH/src/github.com/hyperledger/fabric
