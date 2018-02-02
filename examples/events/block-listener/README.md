# Deprecation
Please note that events API available in Hyperledger Fabric v1.0.0 will be deprecated in favour of new
events delivery API

```proto
service Deliver {
    // deliver first requires an Envelope of type ab.DELIVER_SEEK_INFO with Payload data as a marshaled orderer.SeekInfo message,
    // then a stream of block replies is received.
    rpc Deliver (stream common.Envelope) returns (stream DeliverResponse) {
    }
    // deliver first requires an Envelope of type ab.DELIVER_SEEK_INFO with Payload data as a marshaled orderer.SeekInfo message,
    // then a stream of **filtered** block replies is received.
    rpc DeliverFiltered (stream common.Envelope) returns (stream DeliverResponse) {
    }
}
```

Please explore `eventsclient` example for demonstration of using new APIs. 

# What is block-listener
block-listener.go connects to a peer in order to receive block and chaincode
events (if there are chaincode events being sent).

# To Run
```sh
1. go build

2. ./block-listener -events-address=<peer-address> -events-from-chaincode=<chaincode-id> -events-mspdir=<msp-directory> -events-mspid=<msp-id>
```
Please note that the default MSP under fabric/sampleconfig will be used if no
MSP parameters are provided.

# Example with the e2e_cli example
The block listener can be used with TLS enabled or disabled. By default,
the e2e_cli example will have TLS enabled. In order to allow the
block-listener sample to connect to peers on e2e_cli example with a TLS
enabled, the easiest way would be to map 127.0.0.1 to the hostname of peer
that you are connecting to, such as peer0.org1.example.com. For example on
\*nix based systems this would be an entry in /etc/hosts file.

If you would prefer to disable TLS, you may do so by setting
CORE_PEER_TLS_ENABLED=***false*** in ``docker-compose-cli.yaml`` and
``base/peer-base.yaml`` as well as
ORDERER_GENERAL_TLS_ENABLED=***false*** in``base/docker-compose-base.yaml``.

Next, run the [e2e_cli example](https://github.com/hyperledger/fabric/tree/master/examples/e2e_cli).

Once the "All in one" command:
```sh
./network_setup.sh up
```
has completed, attach the event client to peer peer0.org1.example.com by doing
the following (assuming you are running block-listener in the host environment)
if TLS is enabled:
```sh
CORE_PEER_TLS_ENABLED=true CORE_PEER_TLS_ROOTCERT_FILE=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt ./block-listener -events-address=peer0.org1.example.com:7053 -events-mspdir=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp -events-mspid=Org1MSP
```

If TLS is disabled, you can simply run:
```sh
./block-listener -events-address=peer0.org1.example.com:7053 -events-mspdir=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp -events-mspid=Org1MSP
```

The event client should output "Event Address: peer0.org1.example.com:7053"
and wait for events.

Exec into the cli container:

```sh
docker exec -it cli bash
```

Next, setup the environment variables for peer0.org1.example.com.
If TLS is enabled:
```sh
CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
CORE_PEER_ADDRESS=peer0.org1.example.com:7051
CORE_PEER_LOCALMSPID="Org1MSP"
ORDERER_CA=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem
```
If TLS is disabled:
```sh
CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
CORE_PEER_ADDRESS=peer0.org1.example.com:7051
CORE_PEER_LOCALMSPID="Org1MSP"
```

Create an invoke transaction. If TLS is enabled:
```sh
peer chaincode invoke -o orderer.example.com:7050 --tls --cafile $ORDERER_CA -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'
```
If TLS is disabled:
```sh
peer chaincode invoke -o orderer.example.com:7050 -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'
```
Now you should see the block content displayed in the terminal running the block
listener.


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
