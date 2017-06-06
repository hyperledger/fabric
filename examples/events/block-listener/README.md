# What is block-listener
block-listener.go will connect to a peer and receive block and chaincode events (if there are chaincode events being set).
Currently, this example only runs with TLS disabled.

# To Run
```sh
1. go build

2. ./block-listener -events-address=< event address > -events-from-chaincode=< chaincode ID > -events-mspdir=< msp directory > -events-mspid=< msp id >
```
Please be noted that if no msp info provided, it uses default MSP under fabric/sampleconfig.
# Example with e2e
Please make sure you have finished running the [e2e_cli example](https://github.com/hyperledger/fabric/tree/master/examples/e2e_cli). Before doing that, don't forget to make sure that TLS has been disabled by setting the  CORE_PEER_TLS_ENABLED=***false*** in ``docker-compose-cli.yaml``, ``base/docker-compose-base.yaml`` and ``base/peer-base.yaml``. 

Suppose you just finished the All-in-one:
```sh
./network_setup.sh up
```
Attach event client to peer peer0.org1.example.com (suppose you are running block-listener in the host environment):
```sh
./block-listener -events-address=127.0.0.1:7053 -events-mspdir=<peer0.org1.example.com's msp directory > -events-mspid=Org1MSP
```

Event client should output "Event Address: 127.0.0.1:7053" and wait for events.

Exec into the cli container:

```sh
docker exec -it cli bash
```
Setup the environment variables for peer0.org1.example.com
```sh
CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
CORE_PEER_ADDRESS=peer0.org1.example.com:7051
CORE_PEER_LOCALMSPID="Org1MSP"
```

Create an invoke transaction:

```sh
peer chaincode invoke -o orderer.example.com:7050 -C $CHANNEL_NAME -n mycc -c '{"Args":["invoke","a","b","10"]}'
```
Now you should see the block content received in events client.


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
