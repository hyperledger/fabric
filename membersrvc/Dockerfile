from hyperledger/fabric-baseimage:latest
# Copy GOPATH src and install Peer
RUN mkdir -p /var/hyperledger/db
WORKDIR $GOPATH/src/github.com/hyperledger/fabric/
COPY . .
WORKDIR membersrvc
RUN pwd
RUN CGO_CFLAGS=" " CGO_LDFLAGS="-lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy" go install && cp $GOPATH/src/github.com/hyperledger/fabric/membersrvc/membersrvc.yaml $GOPATH/bin
# RUN CGO_CFLAGS=" " CGO_LDFLAGS="-lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy" go install
