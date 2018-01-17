# Events client
This sample client demonstrates how to connect to a peer to receive block
events. Block events come in the form of either full blocks as they have been
committed to the ledger or filtered blocks (a minimal set of information about
the block) which includes the transaction ids, transaction statuses, and any
chaincode events associated with the transaction.

# Events service interface
Starting with v1.1, two new event services are available:

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

This sample demonstrates connecting to both of these services.

# General use
```sh
cd fabric/examples/events/eventsclient
go build
```
You will see the executable **eventsclient** if there are no compilation errors.

Next, to start receiving block events from a peer with TLS enabled, run the
following command:

```sh
CORE_PEER_LOCALMSPID=<msp-id> CORE_PEER_MSPCONFIGPATH=<path to MSP folder> ./eventsclient -channelID=<channel-id> -filtered=<true or false> -tls=true -clientKey=<path to the client key> -clientCert=<path to the client TLS certificate> -rootCert=<path to the server root CA certificate>
```

If the peer is not using TLS you can run:

```bash
CORE_PEER_LOCALMSPID=<msp-id> CORE_PEER_MSPCONFIGPATH=<path to MSP folder> ./eventsclient -channelID=<channel-id> -filtered=<true or false> -tls=false
```

The peer will begin delivering block events and print the output to the console.

# Example with the e2e_cli example
The events client sample can be used with TLS enabled or disabled. By default,
the e2e_cli example will have TLS enabled. In order to allow the events client
to connect to peers created by the e2e_cli example with TLS enabled, the easiest
way would be to map `127.0.0.1` to the hostname of the peer that you are
connecting to, such as `peer0.org1.example.com`. For example on \*nix based
systems this would be an entry in `/etc/hosts` file.

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
the following:

* If TLS is enabled:
  * to receive full blocks:
```sh
CORE_PEER_LOCALMSPID=Org1MSP CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/peers/peer0.Org1.example.com/msp ./eventsclient -server=peer0.org1.example.com:7051 -channelID=mychannel -filtered=false -tls=true -clientKey=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@Org1.example.com/tls/client.key -clientCert=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@Org1.example.com/tls/client.crt -rootCert=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@Org1.example.com/tls/ca.crt
```

  * to receive filtered blocks:
```sh
CORE_PEER_LOCALMSPID=Org1MSP CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/peers/peer0.Org1.example.com/msp ./eventsclient -server=peer0.org1.example.com:7051 -channelID=mychannel -filtered=true -tls=true -clientKey=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@Org1.example.com/tls/client.key -clientCert=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@Org1.example.com/tls/client.crt -rootCert=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/users/Admin@Org1.example.com/tls/ca.crt
```

* If TLS is disabled:
  * to receive full blocks:
```sh
CORE_PEER_LOCALMSPID=Org1MSP CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/peers/peer0.Org1.example.com/msp ./eventsclient -server=peer0.org1.example.com:7051 -channelID=mychannel -filtered=false -tls=false
```

  * to receive filtered blocks:
```sh
CORE_PEER_LOCALMSPID=Org1MSP CORE_PEER_MSPCONFIGPATH=$GOPATH/src/github.com/hyperledger/fabric/examples/e2e_cli/crypto-config/peerOrganizations/org1.example.com/peers/peer0.Org1.example.com/msp ./eventsclient -server=peer0.org1.example.com:7051 -channelID=mychannel -filtered=true -tls=false
```

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
