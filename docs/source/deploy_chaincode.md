# Deploying a smart contract on a channel

Applications interact with the blockchain ledger through smart contracts. In Hyperledger Fabric, smart contracts are deployed in packages referred to as chaincode. Chaincode needs to be installed on all of the peers will use a smart contract to endorse a transaction or query the data on the ledger. Once a chaincode has been deployed to a channel, channel members can use the smart contract to create and update assets on the channel ledger.

Chaincode is deployed to a channels using the Fabric chaincode lifecycle. The Fabric chaincode lifecycle is a process that allows multiple organizations to agree on how a chaincode will be operated before it can be used to submit transactions. For example, while an endorsement policy is used to decide which organizations need to endorse a valid transaction, channel members need to use the Fabric chaincode lifecycle to decide what the endorsment polcy of the chaincode before it is deployed.

This tutorial describes how to deploy a chaincode to a channel using the [peer lifecycle chaincode commands](./commands/peerlifecycle.html) provided by the peer CLI. We will first bring up the Fabric test network and then use peer CLI to deploy a chaincode to a channel on the network. For a more in depth overview about how to deploy and manage a chaincode on a channel, see [Chaincode for Operators](./chaincode4noah.html).

## Start the network

We will use the Fabric test network to create a channel that we will use to deploy the chaincode. Use the following command to navigate to the test network within your local clone of the `fabric-samples` repository:
```
cd fabric-samples/test-network
```
For the sake of this tutorial, we want to operate from a known initial state. The following command will kill any active or stale docker containers and remove previously generated artifacts.
```
./network.sh down
```
You can then use the following command to start the Fabric test network:
```
./network.sh up createChannel
```

The test network has two organizations, Org1 and Org1, with one peer each. The `createChannel` command creates a channel named ``mychannel`` that has Org1 and Org2 as members, and joins both peers to the channel. If the network and the channel were created successfully, you can see the following message printed in the logs:
```
========= Channel successfully joined ===========
```

We can now use the Peer CLI to deploy the `fabcar` chaincode to the channel using the following steps:


- [Step one: Package the smart contract](#packaging-the-smart-contract)
- [Step two: Install the chaincode package](#install-the-chaincode-package)
- [Step three: Approve a chaincode definition](#approve-a-chaincode-definition)
- [Step four: Committing the chaincode definition to the channel](#committing-the-chaincode-definition-to-the-channel)

## Package the smart contract

If you have not run through the tutorial before, you will need to vendor the
chaincode dependencies before we can deploy it to the network. Run the
following commands:

.. code:: bash

    cd ../chaincode/marbles02_private/go
    GO111MODULE=on go mod vendor
    cd ../../../test-network

Assuming you have started the test network, copy and paste the following environment variables in your CLI to interact with the network and operate as the Org1 admin. Make sure that you are in the `test-network` directory.

```
export PATH=${PWD}/../bin:${PWD}:$PATH
export FABRIC_CFG_PATH=$PWD/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

We need to package the chaincode before it can be installed on our peers. For each package you create, you need to provide a chaincode package label as a description of the chaincode. Use the following commands to package a sample Go, Node.js or Java chaincode.

**Golang**

```
# this packages a Golang chaincode.
# make note of the --lang flag to indicate "golang" chaincode
# for go chaincode --path takes the relative path from $GOPATH/src
# The --label flag is used to create the package label
peer lifecycle chaincode package mycc.tar.gz --path github.com/hyperledger/fabric-samples/chaincode/abstore/go/ --lang golang --label mycc_1
```
**Node.js**

```
# this packages a Node.js chaincode
# make note of the --lang flag to indicate "node" chaincode
# for node chaincode --path takes the absolute path to the node.js chaincode
# The --label flag is used to create the package label
peer lifecycle chaincode package mycc.tar.gz --path /opt/gopath/src/github.com/hyperledger/fabric-samples/chaincode/abstore/node/ --lang node --label mycc_1
```
**Java**

```
# this packages a java chaincode
# make note of the --lang flag to indicate "java" chaincode
# for java chaincode --path takes the absolute path to the java chaincode
# The --label flag is used to create the package label
peer lifecycle chaincode package mycc.tar.gz --path /opt/gopath/src/github.com/hyperledger/fabric-samples/chaincode/abstore/java/ --lang java --label mycc_1
```

Each of the above commands will create a chaincode package named ``mycc.tar.gz``, which we can use to install the chaincode on our peers.

## Install the chaincode package

The chaincode needs to be installed on every peer that will endorse a transaction. Because we are going to set the endorsement policy of ``mycc`` to require endorsements from both Org1 and Org2, we need to install the chaincode on both peers in the test network:

- peer0.org1.example.com
- peer0.org2.example.com

Set the following environment variables to operate as the Org1 admin, and set the address of `peer0.org1.example.com`:
```
export PATH=${PWD}/../bin:${PWD}:$PATH
export FABRIC_CFG_PATH=$PWD/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

Issue the following command to install the package on Org1's peer:

```
peer lifecycle chaincode install mycc.tar.gz
```

A successful install command will return a chaincode package identifier. You should see output similar to the following:

```
2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 001 Installed remotely: response:<status:200 payload:"\nEmycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173" >
2019-03-13 13:48:53.691 UTC [cli.lifecycle.chaincode] submitInstallProposal -> INFO 002 Chaincode code package identifier: mycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173
```

You can also find the chaincode package identifier by querying your peer for information about the packages you have installed.
```
# this returns the details of the chaincode packages installed on your peers
peer lifecycle chaincode queryinstalled
```

The command above will return the same package identifier as the install command. You should see output similar to the following:
```
Get installed chaincodes on peer:
Package ID: mycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173, Label: mycc_1
```

We are going to need the package ID for future commands, so let's go ahead and save it as an environment variable. Paste the package ID returned by the `peer lifecycle chaincode queryinstalled` command into the command below. The package ID may not be the same for all users, so you need to complete this step using the package ID returned from your console.


```
# Save the package ID as an environment variable.
CC_PACKAGE_ID=mycc_1:3a8c52d70c36313cfebbaf09d8616e7a6318ababa01c7cbe40603c373bcfe173
```



Modify the following four environment variables to issue the install command as Org2:

```
# Environment variables to operate as Org2
CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
CORE_PEER_ADDRESS=peer0.org2.example.com:9051
CORE_PEER_LOCALMSPID="Org2MSP"
CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
```

Now install the chaincode package onto peer0 of Org2. The following command will install the chaincode and return same identifier as the install command we issued as Org1.

```
# this installs a chaincode package on your peer
peer lifecycle chaincode install mycc.tar.gz
```

## Approve a chaincode definition

After you install the package, you need to approve a chaincode definition for your organization. The chaincode definition includes the important parameters of chaincode governance, including the chaincode name and version. The definition also includes the package identifier used to associate the chaincode package installed on your peers with a chaincode definition approved by your organization.

Because we set the environment variables to operate as Org2, we can use the following command to approve a definition of the ``mycc`` chaincode for Org2. The approval is distributed to peers within each organization, so the command does not need to target every peer within an organization.

```
# this approves a chaincode definition for your org
# make note of the --package-id flag that provides the package ID
# use the --init-required flag to request the ``Init`` function be invoked to initialize the chaincode
peer lifecycle chaincode approveformyorg --channelID $CHANNEL_NAME --name mycc --version 1.0 --init-required --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

We could have provided a ``--signature-policy`` or ``--channel-config-policy`` argument to the command above to set the chaincode endorsement policy. The endorsement policy specifies how many peers belonging to different channel members need to validate a transaction against a given chaincode. Because we did not set a policy, the definition of ``mycc`` will use the default endorsement policy, which requires that a transaction be endorsed by a majority of channel members present when the transaction is submitted. This implies that if new organizations are added to or removed from the channel, the endorsement policy
is updated automatically to require more or fewer endorsements. In this tutorial, the default policy will require an endorsement from a peer belonging to Org1 **AND** Org2 (i.e. two endorsements). See the :doc:`endorsement-policies` documentation for more details on policy implementation.

All organizations need to agree on the definition before they can use the chaincode. Modify the following four environment variables to operate as Org1:

```
# Environment variables for Org1

CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
CORE_PEER_ADDRESS=peer0.org1.example.com:7051
CORE_PEER_LOCALMSPID="Org1MSP"
CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
```

You can now approve a definition for the ``mycc`` chaincode as Org1. Chaincode is approved at the organization level. You can issue the command once even if you have multiple peers.

```
# this defines a chaincode for your org
# make note of the --package-id flag that provides the package ID
# use the --init-required flag to request the Init function be invoked to initialize the chaincode
peer lifecycle chaincode approveformyorg --channelID $CHANNEL_NAME --name mycc --version 1.0 --init-required --package-id $CC_PACKAGE_ID --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

## Committing the chaincode definition to the channel

Once a sufficient number of channel members have approved a chaincode definition, one member can commit the definition to the channel. By default a majority of channel members need to approve a definition before it can be committed. It is possible to check whether the chaincode definition is ready to be committed and view the current approvals by organization by issuing the following query:

```
# the flags used for this command are identical to those used for approveformyorg
# except for --package-id which is not required since it is not stored as part of
# the definition
peer lifecycle chaincode checkcommitreadiness --channelID $CHANNEL_NAME --name mycc --version 1.0 --init-required --sequence 1 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem --output json
```

The command will produce as output a JSON map showing if the organizations in the channel have approved the chaincode definition provided in the checkcommitreadiness command. In this case, given that both organizations have approved, we obtain:

```
    {
            "Approvals": {
                    "Org1MSP": true,
                    "Org2MSP": true
            }
    }
```

Since both channel members have approved the definition, we can now commit it to the channel using the following command. You can issue this command as either Org1 or Org2. Note that the transaction targets peers in Org1 and Org2 to collect endorsements.
```
# this commits the chaincode definition to the channel

peer lifecycle chaincode commit -o orderer.example.com:7050 --channelID $CHANNEL_NAME --name mycc --version 1.0 --sequence 1 --init-required --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
```

You can use the ``peer lifecycle chaincode querycommitted`` command to check if
the chaincode definition you have approved has already been committed to the
channel.

```
# use the --name flag to select the chaincode whose definition you want to query
peer lifecycle chaincode querycommitted --channelID mychannel --name fabcar --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

A successful command will return information about the committed definition:


```
Committed chaincode definition for chaincode 'fabcar' on channel 'mychannel':
Version: 1, Sequence: 1, Endorsement Plugin: escc, Validation Plugin: vscc
```

## Invoking the chaincode

After a chaincode definition has been committed to a channel, we are ready to invoke the chaincode and start interacting with the ledger. We requested the execution of the ``Init`` function in the chaincode definition using the ``--init-required`` flag. As a result, we need to pass the ``--isInit`` flag to its first invocation and supply the arguments to the ``Init`` function. Issue the following command to initialize the chaincode and put the initial data on the ledger.

```
# be sure to set the -C and -n flags appropriately
# use the --isInit flag if you are invoking an Init function

peer chaincode invoke -o orderer.example.com:7050 --isInit --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n mycc --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["Init","a","100","b","100"]}' --waitForEvent
```

The first invoke will start the chaincode container. We may need to wait for the container to start. Node.js images will take longer.

Let's query the chaincode to make sure that the container was properly started and the state DB was populated. The syntax for query is as follows:

```
# be sure to set the -C and -n flags appropriately

peer chaincode query -C $CHANNEL_NAME -n mycc -c '{"Args":["query","a"]}'
```

Now let’s move ``10`` from ``a`` to ``b``. This transaction will cut a new block and update the state DB. The syntax for invoke is as follows:

```
# be sure to set the -C and -n flags appropriately

peer chaincode invoke -o orderer.example.com:7050 --tls true --cafile /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem -C $CHANNEL_NAME -n mycc --peerAddresses peer0.org1.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt --peerAddresses peer0.org2.example.com:9051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt -c '{"Args":["invoke","a","b","10"]}' --waitForEvent
```

Let's confirm that our previous invocation executed properly. We initialized the key ``a`` with a value of ``100`` and just removed ``10`` with our previous invocation. Therefore, a query against ``a`` should return ``90``. The syntax for query is as follows.

```
# be sure to set the -C and -n flags appropriately
peer chaincode query -C $CHANNEL_NAME -n mycc -c '{"Args":["query","a"]}'
```

We should see the following:
```
Query Result: 90
```

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/) -->
