FROM hyperledger/fabric-peer:latest
# setup the chaincode sample
WORKDIR $GOPATH/src/github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02
RUN go build
# build the node SDK
WORKDIR $GOPATH/src/github.com/hyperledger/fabric/sdk/node
RUN make all
# now switch to the sample node app location when the shell is opened in the docker
WORKDIR $GOPATH/src/github.com/hyperledger/fabric/examples/sdk/node
# install the hfc locally for use by the application
RUN npm install $GOPATH/src/github.com/hyperledger/fabric/sdk/node
