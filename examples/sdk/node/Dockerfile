FROM hyperledger/fabric-peer:latest
# setup the chaincode sample
WORKDIR $GOPATH/src/github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02
RUN go build
WORKDIR $GOPATH/src/github.com/hyperledger/fabric/examples/sdk/node
# install the hfc locally for use by the application
RUN npm install hfc
