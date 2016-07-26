### UTXO Chaincode

The UTXO example chaincode contains a single invocation function named `execute`. This function accepts BASE64 encoded transactions from the Bitcoin network. This chaincode will parse the transactions and pass the transaction components to the Bitcoin libconsensus C library for script verification.

The purpose of this chaincode is to

1. Demonstrate how the world state can be used to store and process unspent transaction outputs (UTXO).

2. Demonstrate how to include and use a C library from within a chaincode.

A client for exercising this chaincode is avilable at https://github.com/srderson/hyperledger-fabric-utxo-client-java.


The following are instructions for building and deploying the UTXO chaincode in Hypereledger Fabric. All commands should be run with vagrant.

First, build the Docker image for the UTXO chaincode.

```
cd $GOPATH/src/github.com/hyperledger/fabric/examples/chaincode/go/utxo/
docker build -t utxo:0.1.0 .
```

Next, modify the `core.yaml` file in the Hyperledger Fabric project to point to the local Docker image that was built in the previous step. In the core.yaml file find `chaincode.golang.Dockerfile` and change it from from `hyperledger/fabric-baseimage` to `utxo:0.1.0`

Start the peer using the following commands
```
peer node start
```

In a second window, deploy the example UTXO chaincode
```
CORE_PEER_ADDRESS=localhost:30303 peer chaincode deploy -p github.com/hyperledger/fabric/examples/chaincode/go/utxo -c '{"Function":"init", "Args": []}'
```
Wait about 30 seconds for the chaincode to be deployed. Output from the window where the peer is running will indicate that this is successful.

Next, find the `image ID` for the deployed chaincode. Run
```
docker images
```
and look for the image ID of the most recently deployed chaincode. The image ID will likely be similar to
```
 dev-jdoe-cbe6be7ed67931b9be2ce31dd833e523702378bef91b29917005f0eaa316b57e268e19696093d48b91076f1134cbf4b06afd78e6afd947133f43cb51bf40b0a4
 ```
 Make a note of this as we'll be using it later.

Stop the running peer.

Build a peer docker image by running the following test. This will allow for easy testing of the chaincode by giving us the ability to reset the database to a clean state.
```
go test github.com/hyperledger/fabric/core/container -run=BuildImage_Peer
```

Using the Docker image that we just built, start a peer within a container in `chaincodedev` mode.
```
docker run -it -p 30303:30303 -p 31315:31315 hyperledger/fabric-peer peer node start --peer-chaincodedev
```


In another window, start UTXO chaincode in a container. The <image ID> refers to the UTXO image ID noted in a prior step.
```
docker run -it <image ID> /bin/bash
```

Build the UTXO chaincode.
```
cd $GOPATH/src/github.com/hyperledger/fabric/examples/chaincode/go/utxo/
go build
CORE_PEER_ADDRESS=172.17.0.2:30303 CORE_CHAINCODE_ID_NAME=utxo ./utxo
```

In another window, deploy the chaincode
```
peer chaincode deploy -n utxo -c '{"Function":"init", "Args": []}'
```

The chaincode is now deployed and ready to accept transactions.
