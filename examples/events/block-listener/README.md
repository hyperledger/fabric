# What is block-listener
block-listener.go connects to a peer in order to receive block and chaincode
events (if there are chaincode events being sent). Currently, this example only
works with TLS disabled in the environment.

# To Run
```sh
1. go build

2. ./block-listener -events-address=<peer-address> -events-from-chaincode=<chaincode-id> -events-mspdir=<msp-directory> -events-mspid=<msp-id>
```
Please note that the default MSP under fabric/sampleconfig will be used if no
MSP parameters are provided.

# Example with the e2e_cli example
In order to use the block listener with the e2e_cli example, make sure that TLS
has been disabled by setting CORE_PEER_TLS_ENABLED=***false*** in
``docker-compose-cli.yaml``, ``base/docker-compose-base.yaml`` and
``base/peer-base.yaml``.

Next, run the [e2e_cli example](https://github.com/hyperledger/fabric/tree/master/examples/e2e_cli).

Once the "All in one" command:
```sh
./network_setup.sh up
```
has completed, attach the event client to peer peer0.org1.example.com by doing
the following (assuming you are running block-listener in the host environment):
```sh
./block-listener -events-address=127.0.0.1:7053 -events-mspdir=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp -events-mspid=Org1MSP
```

The event client should output "Event Address: 127.0.0.1:7053" and wait for
events.

Exec into the cli container:

```sh
docker exec -it cli bash
```
Setup the environment variables for peer0.org1.example.com
```sh
CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
CORE_PEER_ADDRESS=peer0.org1.example.com:7051
CORE_PEER_LOCALMSPID="Org1MSP"
```

Create an invoke transaction:

```sh
peer chaincode invoke -o orderer.example.com:7050 -C $CHANNEL_NAME -n mycc -c '{"Args":["invoke","a","b","10"]}'
```
Now you should see the block content displayed in the terminal running the block
listener.


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
