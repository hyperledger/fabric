# Creating a new channel

You can use this tutorial to learn how to create new channels using the [configtxgen](../commands/configtxgen.html) CLI tool and then use the [peer channel](../commands/peerchannel.html) commands to join a channel with your peers. While this tutorial will leverage the Fabric test network to create the new channel, the steps in this tutorial can also be used by network operators in a production environment.

In the process of creating the channel, this tutorial will take you through the following steps and concepts:

- [Setting up the configtxgen tool](#setting-up-the-configtxgen-tool)
- [Using the configtx.yaml file](#the-configtx-yaml-file)
- [The orderer system channel](#the-orderer-system-channel)
- [Creating an application channel](#creating-an-application-channel)
- [Joining peers to the channel](#join-peers-to-the-channel)
- [Setting anchor peers](#set-anchor-peers)

## Setting up the configtxgen tool

Channels are created by building a channel creation transaction and submitting the transaction to the ordering service. The channel creation transaction specifies the initial configuration of the channel and is used by the ordering service to write the channel genesis block. While it is possible to build the channel creation transaction file manually, it is easier to use the [configtxgen](../commands/configtxgen.html) tool. The tool works by reading a `configtx.yaml` file that defines the configuration of your channel, and then writing the relevant information into the channel creation transaction. Before we discuss the `configtx.yaml` file in the next section, we can get started by downloading and setting up the `configtxgen` tool.

You can download the `configtxgen` binaries by following the steps to [install the samples, binaries and Docker images](../install.html). `configtxgen` will be downloaded to the `bin` folder of your local clone of the `fabric-samples` repository along with other Fabric tools.

For the purposes of this tutorial, we will want to operate from the `test-network` directory inside `fabric-samples`. Navigate to that directory using the following command:
```
cd fabric-samples/test-network
```
We will operate from the `test-network` directory for the remainder of the tutorial. Use the following command to add the configtxgen tool to your CLI path:
```
export PATH=${PWD}/../bin:$PATH
```

In order to use `configtxgen`, you need to the set the `FABRIC_CFG_PATH` environment variable to the path of the directory that contains your local copy of the `configtx.yaml` file. For this tutorial, we will reference the `configtx.yaml` used to setup the Fabric test network in the `configtx` folder:
```
export FABRIC_CFG_PATH=${PWD}/configtx
```

You can check that you can are able to use the tool by printing the `configtxgen` help text:
```
configtxgen --help
```


## The configtx.yaml file

The `configtx.yaml` file specifies the **channel configuration** of new channels. The information that is required to build the channel configuration is specified in a readable and editable form in the `configtx.yaml` file. The `configtxgen` tool uses the channel profiles defined in the `configtx.yaml` file to create the channel configuration and write it to the [protobuf format](https://developers.google.com/protocol-buffers) that can be read by Fabric.

You can find the `configtx.yaml` file that is used to deploy the test network in the `configtx` folder in the `test-network` directory. The file contains the following information that we will use to create our new channel:

- **Organizations:** The organizations that can become members of your channel. Each organization has a reference to the cryptographic material that is used to build the [channel MSP](../membership/membership.html).
- **Ordering service:** Which ordering nodes will form the ordering service of the network, and consensus method they will use to agree to a common order of transactions. The file also contains the organizations that will become the ordering service administrators.
- **Channel policies** Different sections of the file work together to define the policies that will govern how organizations interact with the channel and which organizations need to approve channel updates. For the purposes of this tutorial, we will use the default policies used by Fabric.
- **Channel profiles** Each channel profile references information from other sections of the `configtx.yaml` file to build a channel configuration. The profiles are used the create the genesis block of the orderer system channel and the channels that will be used by peer organizations. To distinguish them from the system channel, the channels used by peer organizations are often referred to as application channels.

  The `configtxgen` tool uses `configtx.yaml` file to create a complete genesis block for the system channel. As a result, the system channel profile needs to specify the full system channel configuration. The channel profile used to create the channel creation transaction only needs to contain the additional configuration information required to create an application channel.

You can visit the [Using configtx.yaml to create a channel genesis block](create_channel_genesis.html) tutorial to learn more about this file. For now, we will return to the operational aspects of creating the channel, though we will reference parts of this file in future steps.

## Start the network

We will use a running instance of the Fabric test network to create the new channel. For the sake of this tutorial, we want to operate from a known initial state. The following command will kill any active containers and remove any previously generated artifacts. Make sure that you are still operating from the `test-network` directory of your local clone of `fabric-samples`.
```
./network.sh down
```
You can then use the following command to start the test network:
```
./network.sh up
```
This command will create a Fabric network with the two peer organizations and the single ordering organization defined in the `configtx.yaml` file. The peer organizations will operate one peer each, while the ordering service administrator will operate a single ordering node. When you run the command, the script will print out logs of the nodes being created:
```
Creating network "net_test" with the default driver
Creating volume "net_orderer.example.com" with default driver
Creating volume "net_peer0.org1.example.com" with default driver
Creating volume "net_peer0.org2.example.com" with default driver
Creating orderer.example.com    ... done
Creating peer0.org2.example.com ... done
Creating peer0.org1.example.com ... done
CONTAINER ID        IMAGE                               COMMAND             CREATED             STATUS                  PORTS                              NAMES
8d0c74b9d6af        hyperledger/fabric-orderer:latest   "orderer"           4 seconds ago       Up Less than a second   0.0.0.0:7050->7050/tcp             orderer.example.com
ea1cf82b5b99        hyperledger/fabric-peer:latest      "peer node start"   4 seconds ago       Up Less than a second   0.0.0.0:7051->7051/tcp             peer0.org1.example.com
cd8d9b23cb56        hyperledger/fabric-peer:latest      "peer node start"   4 seconds ago       Up 1 second             7051/tcp, 0.0.0.0:9051->9051/tcp   peer0.org2.example.com
```

Our instance of the test network was deployed without creating an application channel. However, the test network script creates the system channel when you issue the `./network.sh up` command. Under the covers, the script uses the `configtxgen` tool and the `configtx.yaml` file to build the genesis block of the system channel.  Because the system channel is used to create other channels, we need to take some time to understand the orderer system channel before we can create an application channel.

## The orderer system channel

The first channel that is created in a Fabric network is the system channel. The system channel defines the set of ordering nodes that form the ordering service and the set of organizations that serve as ordering service administrators.

The system channel also includes the organizations that are members of blockchain [consortium](../glossary.html#consortium).
The consortium is a set of peer organizations that belong to the system channel, but are not administrators of the ordering service. Consortium members have the ability to create new channels and include other consortium organizations as channel members.

The genesis block of the system channel is required to deploy a new ordering service. The test network script already created the system channel genesis block when you issued the `./network.sh up` command. The genesis block was used to deploy the single ordering node, which used the block to create the system channel and form the ordering service of the network. If you examine the output of the `./network.sh` script, you can find the command that created the genesis block in your logs:
```
configtxgen -profile TwoOrgsOrdererGenesis -channelID system-channel -outputBlock ./system-genesis-block/genesis.block
```

The `configtxgen` tool used the `TwoOrgsOrdererGenesis` channel profile from `configtx.yaml` to write the genesis block and store it in the `system-genesis-block` folder. You can see the `TwoOrgsOrdererGenesis` profile below:
```yaml
TwoOrgsOrdererGenesis:
    <<: *ChannelDefaults
    Orderer:
        <<: *OrdererDefaults
        Organizations:
            - *OrdererOrg
        Capabilities:
            <<: *OrdererCapabilities
    Consortiums:
        SampleConsortium:
            Organizations:
                - *Org1
                - *Org2
```

The `Orderer:` section of the profile creates the single node Raft ordering service used by the test network, with the `OrdererOrg` as the ordering service administrator. The `Consortiums` section of the profile creates a consortium of peer organizations named `SampleConsortium:`. Both peer organizations, Org1 and Org2, are members of the consortium. As a result, we can include both organizations in new channels created by the test network. If we wanted to add another organization as a channel member without adding that organization to the consortium, we would first need to create the channel with Org1 and Org2, and then add the other organization by [updating the channel configuration](../channel_update_tutorial.html).

## Creating an application channel

Now that we have deployed the nodes of the network and created the orderer system channel using the `network.sh` script, we can start the process of creating a new channel for our peer organizations. We have already set the environment variables that are required to use the `configtxgen` tool. Run the following command to create a channel creation transaction for `channel1`:
```
configtxgen -profile TwoOrgsChannel -outputCreateChannelTx ./channel-artifacts/channel1.tx -channelID channel1
```

The `-channelID` will be the name of the future channel. Channel names must be all lower case, less than 250 characters long and match the regular expression ``[a-z][a-z0-9.-]*``. The command uses the uses the `-profile` flag to reference the `TwoOrgsChannel:` profile from `configtx.yaml` that is used by the test network to create application channels:
```yaml
TwoOrgsChannel:
    Consortium: SampleConsortium
    <<: *ChannelDefaults
    Application:
        <<: *ApplicationDefaults
        Organizations:
            - *Org1
            - *Org2
        Capabilities:
            <<: *ApplicationCapabilities
```

The profile references the name of the `SampleConsortium` from the system channel, and includes both peer organizations from the consortium as channel members. Because the system channel is used as a template to create the application channel, the ordering nodes defined in the system channel become the default [consenter set](../glossary.html#consenter-set) of the new channel, while the administrators of the ordering service become the orderer administrators of the channel. Ordering nodes and ordering organizations can be added or removed from the consenter set using channel updates.

If the command successful, you will see logs of `configtxgen` loading the `configtx.yaml` file and printing a channel creation transaction:
```
2020-03-11 16:37:12.695 EDT [common.tools.configtxgen] main -> INFO 001 Loading configuration
2020-03-11 16:37:12.738 EDT [common.tools.configtxgen.localconfig] Load -> INFO 002 Loaded configuration: /Usrs/fabric-samples/test-network/configtx/configtx.yaml
2020-03-11 16:37:12.740 EDT [common.tools.configtxgen] doOutputChannelCreateTx -> INFO 003 Generating new channel configtx
2020-03-11 16:37:12.789 EDT [common.tools.configtxgen] doOutputChannelCreateTx -> INFO 004 Writing new channel tx
```

We can use the `peer` CLI to submit the channel creation transaction to the ordering service. To use the `peer` CLI, we need to set the `FABRIC_CFG_PATH` to the `core.yaml` file located in the `fabric-samples/config` directory. Set the `FABRIC_CFG_PATH` environment variable by running the following command:
```
export FABRIC_CFG_PATH=$PWD/../config/
```

Before the ordering service creates the channel, the ordering service will check the permission of the identity that submitted the request. By default, only admin identities of organizations that belong to the system channel consortium can create a new channel. Issue the commands below to operate the `peer` CLI as the admin user from Org1:
```
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

You can now create the channel by using the following command:
```
peer channel create -o localhost:7050  --ordererTLSHostnameOverride orderer.example.com -c channel1 -f ./channel-artifacts/channel1.tx --outputBlock ./channel-artifacts/channel1.block --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

The command above provides the path to the channel creation transaction file using the `-f` flag and uses the `-c` flag to specify the channel name. The `-o` flag is used to select the ordering node that will be used to create the channel. The `--cafile` is the path to the TLS certificate of the ordering node. When you run the `peer channel create` command, the `peer` CLI will generate the following response:
```
2020-03-06 17:33:49.322 EST [channelCmd] InitCmdFactory -> INFO 00b Endorser and orderer connections initialized
2020-03-06 17:33:49.550 EST [cli.common] readBlock -> INFO 00c Received block: 0
```
Because we are using a Raft ordering service, you may get some status unavailable messages that you can safely ignore. The command will return the genesis block of the new channel to the location specified by the `--outputBlock` flag.

## Join peers to the channel

After the channel has been created, we can join the channel with our peers. Organizations that are members of the channel can fetch the channel genesis block from the ordering service using the [peer channel fetch](../commands/peerchannel.html#peer-channel-fetch) command. The organization can then use the genesis block to join the peer to the channel using the [peer channel join](../commands/peerchannel.html#peer-channel-join) command. Once the peer is joined to the channel, the peer will build the blockchain ledger by retrieving the other blocks on the channel from the ordering service.

Since we are already operating the `peer` CLI as the Org1 admin, let's join the Org1 peer to the channel. Since Org1 submitted the channel creation transaction, we already have the channel genesis block on our file system. Join the Org1 peer to the channel using the command below.
```
peer channel join -b ./channel-artifacts/channel1.block
```

The `CORE_PEER_ADDRESS` environment variable has been set to target ``peer0.org1.example.com``. A successful command will generate a response from ``peer0.org1.example.com`` joining the channel:
```
2020-03-06 17:49:09.903 EST [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
2020-03-06 17:49:10.060 EST [channelCmd] executeJoin -> INFO 002 Successfully submitted proposal to join channel
```

You can verify that the peer has joined the channel using the [peer channel getinfo](../commands/peerchannel.html#peer-channel-getinfo) command:
```
peer channel getinfo -c channel1
```
The command will list the block height of the channel and the hash of the most recent block. Because the genesis block is the only block on the channel, the height of the channel will be 1:
```
2020-03-13 10:50:06.978 EDT [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
Blockchain info: {"height":1,"currentBlockHash":"kvtQYYEL2tz0kDCNttPFNC4e6HVUFOGMTIDxZ+DeNQM="}
```

We can now join the Org2 peer to the channel. Set the following environment variables to operate the `peer` CLI as the Org2 admin. The environment variables will also set the Org2 peer, ``peer0.org1.example.com``, as the target peer.
```
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org2MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_ADDRESS=localhost:9051
```

While we still have the channel genesis block on our file system, in a more realistic scenario, Org2 would have the fetch the block from the ordering service. As an example, we will use the `peer channel fetch` command to get the genesis block for Org2:
```
peer channel fetch 0 ./channel-artifacts/channel_org2.block -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com -c channel1 --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

The command uses `0` to specify that it needs to fetch the genesis block that is required to join the channel. If the command is successful, you should see the following output:
```
2020-03-13 11:32:06.309 EDT [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
2020-03-13 11:32:06.336 EDT [cli.common] readBlock -> INFO 002 Received block: 0
```

The command returns the channel genesis block and names it `channel_org2.block` to distinguish it from the block pulled by org1. You can now use the block to join the Org2 peer to the channel:
```
peer channel join -b ./channel-artifacts/channel_org2.block
```

## Set anchor peers

After an organizations has joined their peers to the channel, they should select at least one of their peers to become an anchor peer. [Anchor peers](../gossip.html#anchor-peers) are required in order to take advantage of features such as private data and service discovery. Each organization should set multiple anchor peers on a channel for redundancy. For more information about gossip and anchor peers, see the [Gossip data dissemination protocol](../gossip.html).

The endpoint information of the anchor peers of each organization is included in the channel configuration. Each channel member can specify their anchor peers by updating the channel. We will use the [configtxlator](../commands/configtxlator.html) tool to update the channel configuration and select an anchor peer for Org1 and Org2. The process for setting an anchor peer is similar to the steps that are required to make other channel updates and provides an introduction to how to use `configtxlator` to [update a channel configuration](../config_update.html). You will also need to install the [jq tool](https://stedolan.github.io/jq/) on your local machine.

We will start by selecting an anchor peer as Org1. The first step is to pull the most recent channel configuration block using the `peer channel fetch` command. Set the following environment variables to operate the `peer` CLI as the Org1 admin:
```
export FABRIC_CFG_PATH=$PWD/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

You can use the following command to fetch the channel configuration:
```
peer channel fetch config channel-artifacts/config_block.pb -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com -c channel1 --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```
Because the most recent channel configuration block is the channel genesis block, you will see the command return block 0 from the channel.
```
2020-04-15 20:41:56.595 EDT [channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
2020-04-15 20:41:56.603 EDT [cli.common] readBlock -> INFO 002 Received block: 0
2020-04-15 20:41:56.603 EDT [channelCmd] fetch -> INFO 003 Retrieving last config block: 0
2020-04-15 20:41:56.608 EDT [cli.common] readBlock -> INFO 004 Received block: 0
```

The channel configuration block was stored in the `channel-artifacts` folder to keep the update process separate from other artifacts. Change into the  `channel-artifacts` folder to complete the next steps:
```
cd channel-artifacts
```
We can now start using the `configtxlator` tool to start working with the channel configuration. The first step is to decode the block from protobuf into a JSON object that can be read and edited. We also strip away the unnecessary block data, leaving only the channel configuration.

```
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
jq .data.data[0].payload.data.config config_block.json > config.json
```

These commands convert the channel configuration block into a streamlined JSON, `config.json`, that will serve as the baseline for our update. Because we don't want to edit this file directly, we will make a copy that we can edit. We will use the original channel config in a future step.
```
cp config.json config_copy.json
```

You can use the `jq` tool to add the Org1 anchor peer to the channel configuration.
```
jq '.channel_group.groups.Application.groups.Org1MSP.values += {"AnchorPeers":{"mod_policy": "Admins","value":{"anchor_peers": [{"host": "peer0.org1.example.com","port": 7051}]},"version": "0"}}' config_copy.json > modified_config.json
```

After this step, we have an updated version of channel configuration in JSON format in the `modified_config.json` file. We can now convert both the original and modified channel configurations back into protobuf format and calculate the difference between them.
```
configtxlator proto_encode --input config.json --type common.Config --output config.pb
configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
configtxlator compute_update --channel_id channel1 --original config.pb --updated modified_config.pb --output config_update.pb
```

The new protobuf named `channel_update.pb` contains the anchor peer update that we need to apply to the channel configuration. We can wrap the configuration update in a transaction envelope to create the channel configuration update transaction.

```
configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json
echo '{"payload":{"header":{"channel_header":{"channel_id":"channel1", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json
configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb
```

We can now use the final artifact, `config_update_in_envelope.pb`, that can be used to update the channel. Navigate back to the `test-network` directory:
```
cd ..
```

We can add the anchor peer by providing the new channel configuration to the `peer channel update` command. Because we are updating a section of the channel configuration that only affects Org1, other channel members do not need to approve the channel update.
```
peer channel update -f channel-artifacts/config_update_in_envelope.pb -c channel1 -o localhost:7050  --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

When the channel update is successful, you should see the following response:
```
2020-01-09 21:30:45.791 UTC [channelCmd] update -> INFO 002 Successfully submitted channel update
```

We can set the anchor peers for Org2. Because we are going through the process a second time, we will go through the steps more quickly. Set the environment variables to operate the `peer` CLI as the Org2 admin:
```
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org2MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_ADDRESS=localhost:9051
```

Pull the latest channel configuration block, which is now the second block on the channel:
```
peer channel fetch config channel-artifacts/config_block.pb -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com -c channel1 --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

Navigate back to the `channel-artifacts` directory:
```
cd channel-artifacts
```

You can then decode and copy the configuration block.
```
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
jq .data.data[0].payload.data.config config_block.json > config.json
cp config.json config_copy.json
```

Add the Org2 peer that is joined to the channel as the anchor peer in the channel configuration:
```
jq '.channel_group.groups.Application.groups.Org2MSP.values += {"AnchorPeers":{"mod_policy": "Admins","value":{"anchor_peers": [{"host": "peer0.org2.example.com","port": 9051}]},"version": "0"}}' config_copy.json > modified_config.json
```

We can now convert both the original and updated channel configurations back into protobuf format and calculate the difference between them.
```
configtxlator proto_encode --input config.json --type common.Config --output config.pb
configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
configtxlator compute_update --channel_id channel1 --original config.pb --updated modified_config.pb --output config_update.pb
```

Wrap the configuration update in a transaction envelope to create the channel configuration update transaction:
```
configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json
echo '{"payload":{"header":{"channel_header":{"channel_id":"channel1", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json
configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb
```

Navigate back to the `test-network` directory.
```
cd ..
```

Update the channel and set the Org2 anchor peer by issuing the following command:
```
peer channel update -f channel-artifacts/config_update_in_envelope.pb -c channel1 -o localhost:7050  --ordererTLSHostnameOverride orderer.example.com --tls --cafile ${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

You can confirm that the channel has been updated successfully by running the `peer channel info` command:
```
peer channel getinfo -c channel1
```
Now that the channel has been updated by adding two channel configuration blocks to the channel genesis block, the height of the channel will have grown to three:
```
Blockchain info: {"height":3,"currentBlockHash":"eBpwWKTNUgnXGpaY2ojF4xeP3bWdjlPHuxiPCTIMxTk=","previousBlockHash":"DpJ8Yvkg79XHXNfdgneDb0jjQlXLb/wxuNypbfHMjas="}
```

## Deploy a chaincode to the new channel

We can confirm that the channel was created successfully by deploying a chaincode to the channel. We can use the `network.sh` script to deploy the Fabcar chaincode to any test network channel. Deploy a chaincode to our new channel using the following command:
```
./network.sh deployCC -c channel1
```

After you run the command, you should see the chaincode being deployed to the channel in your logs. The chaincode is invoked to add data to the channel ledger and then queried.
```
[{"Key":"CAR0","Record":{"make":"Toyota","model":"Prius","colour":"blue","owner":"Tomoko"}},
{"Key":"CAR1","Record":{"make":"Ford","model":"Mustang","colour":"red","owner":"Brad"}},
{"Key":"CAR2","Record":{"make":"Hyundai","model":"Tucson","colour":"green","owner":"Jin Soo"}},
{"Key":"CAR3","Record":{"make":"Volkswagen","model":"Passat","colour":"yellow","owner":"Max"}},
{"Key":"CAR4","Record":{"make":"Tesla","model":"S","colour":"black","owner":"Adriana"}},
{"Key":"CAR5","Record":{"make":"Peugeot","model":"205","colour":"purple","owner":"Michel"}},
{"Key":"CAR6","Record":{"make":"Chery","model":"S22L","colour":"white","owner":"Aarav"}},
{"Key":"CAR7","Record":{"make":"Fiat","model":"Punto","colour":"violet","owner":"Pari"}},
{"Key":"CAR8","Record":{"make":"Tata","model":"Nano","colour":"indigo","owner":"Valeria"}},
{"Key":"CAR9","Record":{"make":"Holden","model":"Barina","colour":"brown","owner":"Shotaro"}}]
===================== Query successful on peer0.org1 on channel 'channel1' =====================
```

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
