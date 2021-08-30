# Create a channel using the test network

Use this tutorial along with the test network to learn how to create a channel genesis block and then create a new application channel that the test network peers can join. Rather than requiring you to set up an orderer, or remove the system channel from an existing orderer, this tutorial leverages the nodes from the Fabric sample test network. Because the test network deploys an ordering service and peers for you, this tutorial focuses solely on the process to create a channel. It is worth noting that the test network includes a `createChannel` subcommand that can be used to create a channel, but this tutorial explains how do it manually, the process that is required when you do not use the test network.

Fabric v2.3 introduces the capability to create a channel without requiring a system channel, removing an extra layer of administration from the process. In this tutorial, we use the [configtxgen](../commands/configtxgen.html) tool to create a channel genesis block and then use the [osnadmin channel](../commands/osnadminchannel.html) command to create the channel.

**Note:**
- If you are _not_ using the test network, you should follow the instructions for [how to deploy an ordering service without a system channel](create_channel_participation.html#deploy-a-new-set-of-orderers). In the Fabric v2.3 test network sample, the single-node ordering service is deployed without a system channel.
- If you prefer to learn how to create a channel on an ordering service that includes the system channel, you should refer to the [Create a channel tutorial](https://hyperledger-fabric.readthedocs.io/en/release-2.2/create_channel/create_channel.html) from Fabric v2.2. In the Fabric v2.2 test network sample, the single-node ordering service is deployed with a system channel.

To create a channel using the test network, this tutorial takes you through the following steps and concepts:
- [Prerequisites](#prerequisites)
- [Step one: Generate the genesis block of the channel](#step-one-generate-the-genesis-block-of-the-channel)
- [Step two: Create the application channel](#step-two-create-the-application-channel)
- [Next steps](#next-steps)

## Before you begin

To run the test network, you need to clone the `fabric-samples`
repository and download the latest production Fabric images. Make sure that you have installed
the [Prerequisites](../prereqs.html) and [Installed the Samples, Binaries, and Docker Images](../install.html).  

**Note:** After you create a channel and join peers to it, you will need to you add anchor peers to the channel, in order for service discovery and private data to work. Instructions on how to set an anchor peer on your channel are included in this tutorial, but require that the [jq tool](https://stedolan.github.io/jq/) is installed on your local machine.

## Prerequisites

### Start the test network

We will use a running instance of the Fabric test network to create the new channel. Because it's important to operate from a known initial state, the following command destroys any active containers and removes any previously generated artifacts. For the purposes of this tutorial, we operate from the `test-network` directory inside `fabric-samples`. If you are not already there, navigate to that directory using the following command:
```
cd fabric-samples/test-network
```
Run the following command to bring down the network:
```
./network.sh down
```
You can then use the following command to start the test network:
```
./network.sh up
```
This command creates a Fabric network with the two peer organizations and the single ordering node ordering organization. The peer organizations will operate one peer each, while the ordering service administrator will operate a single ordering node. When you run the command, the script prints out the nodes being created:
```
Creating network "fabric_test" with the default driver
Creating volume "net_orderer.example.com" with default driver
Creating volume "net_peer0.org1.example.com" with default driver
Creating volume "net_peer0.org2.example.com" with default driver
Creating peer0.org2.example.com ... done
Creating orderer.example.com    ... done
Creating peer0.org1.example.com ... done
Creating cli                    ... done
CONTAINER ID   IMAGE                               COMMAND             CREATED         STATUS                  PORTS                                            NAMES
1667543b5634   hyperledger/fabric-tools:latest     "/bin/bash"         1 second ago    Up Less than a second                                                    cli
b6b117c81c7f   hyperledger/fabric-peer:latest      "peer node start"   2 seconds ago   Up 1 second             0.0.0.0:7051->7051/tcp                           peer0.org1.example.com
703ead770e05   hyperledger/fabric-orderer:latest   "orderer"           2 seconds ago   Up Less than a second   0.0.0.0:7050->7050/tcp, 0.0.0.0:7053->7053/tcp   orderer.example.com
718d43f5f312   hyperledger/fabric-peer:latest      "peer node start"   2 seconds ago   Up 1 second             7051/tcp, 0.0.0.0:9051->9051/tcp                 peer0.org2.example.com
```

Notice that the peers are running on ports `7051` and `9051`, while the orderer is running on port `7050`. We will use these ports in subsequent commands.  

By default, when you start the test network, it does not contain any channels. The following instructions demonstrate how to add a channel that is named `channel1` to this network.

### Set up the configtxgen tool

Channels are created by generating a channel creation transaction in a genesis block, and then passing that genesis block to an ordering service node in a join request. The channel creation transaction specifies the initial configuration of the channel and can be created by the [configtxgen](../commands/configtxgen.html) tool. The tool reads the `configtx.yaml` file that defines the configuration of our channel, and then writes the relevant information into the channel creation transaction and outputs a genesis block including the channel creation transaction. When you [installed Fabric](../install.html), the `configtxgen` tool was installed in the `fabric-samples\bin` directory for you.

Ensure that you are still operating from the `test-network` directory of your local clone of `fabric-samples` and run this command:

```
export PATH=${PWD}/../bin:$PATH
```

Next, before you can use `configtxgen`, you need to the set the `FABRIC_CFG_PATH` environment variable to the location of the test network folder that contains the `configtx.yaml` file. Because we are using the test network, we reference the `configtx` folder:
```
export FABRIC_CFG_PATH=${PWD}/configtx
```

Now verify that you can use the tool by printing the `configtxgen` help text:
```
configtxgen --help
```

### The configtx.yaml file

For the test network, the `configtxgen` tool uses the channel profiles that are defined in the `configtxt\configtx.yaml` file to create the channel configuration and write it to the [protobuf format](https://developers.google.com/protocol-buffers) that can be read by Fabric.

This `configtx.yaml` file contains the following information that we will use to create our new channel:

- **Organizations:** The peer and ordering organizations that can become members of your channel. Each organization has a reference to the cryptographic material that is used to build the [channel MSP](../membership/membership.html).
- **Ordering service:** Which ordering nodes will form the ordering service of the network, and consensus method they will use to agree to a common order of transactions. This section also defines the ordering nodes that are part of the ordering service consenter set. In the test network sample, there is only a single ordering node, but in a production network we recommend **five** ordering nodes to allow for two ordering nodes to go down and still maintain consensus.
    ```
    EtcdRaft:
        Consenters:
        - Host: orderer.example.com
          Port: 7050
          ClientTLSCert: ../organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.crt
          ServerTLSCert: ../organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.crt
    ```
- **Channel policies** Different sections of the file work together to define the policies that will govern how organizations interact with the channel and which organizations need to approve channel updates. For the purposes of this tutorial, we will use the default policies used by Fabric.
- **Channel profiles** Each channel profile references information from other sections of the `configtx.yaml` file to build a channel configuration. The profiles are used to create the genesis block of application channel. Notice that the `configtx.yaml` file in the test network includes a single profile named `TwoOrgsApplicationGenesis` that we will use to generate the create channel transaction.
    ```yaml
    TwoOrgsApplicationGenesis:
        <<: *ChannelDefaults
        Orderer:
            <<: *OrdererDefaults
            Organizations:
                - *OrdererOrg
            Capabilities: *OrdererCapabilities
        Application:
            <<: *ApplicationDefaults
            Organizations:
                - *Org1
                - *Org2
            Capabilities: *ApplicationCapabilities
    ```

The profile includes both peer organizations, `Org1` and `Org2` as well as the ordering organization `OrdererOrg`. Additional ordering nodes and ordering organizations can be added or removed from the consenter set at a later time using a channel update transaction.

Want to learn more about this file and how to build your own channel application profiles? Visit [Using configtx.yaml to create a channel genesis block](create_channel_config.html) tutorial for more details. For now, we will return to the operational aspects of creating the channel, though we will reference parts of this file in future steps.

## Step one: Generate the genesis block of the channel

Because we have started the Fabric test network, we are ready to create a new channel. We have already set the environment variables that are required to use the `configtxgen` tool.   

Run the following command to create the channel genesis block for `channel1`:
```
configtxgen -profile TwoOrgsApplicationGenesis -outputBlock ./channel-artifacts/channel1.block -channelID channel1
```

- **`-profile`**: The command uses the `-profile` flag to reference the `TwoOrgsApplicationGenesis:` profile from `configtx.yaml` that is used by the test network to create application channels.
- **`-outputBlock`**: The output of this command is the channel genesis block that is written to `-outputBlock ./channel-artifacts/channel1.block`.
- **`-channelID`**: The `-channelID` parameter will be the name of the future channel. You can specify any name you want for your channel but for illustration purposes in this tutorial we use `channel1`. Channel names must be all lowercase, fewer than 250 characters long and match the regular expression ``[a-z][a-z0-9.-]*``.

When the command is successful, you can see the logs of `configtxgen` loading the `configtx.yaml` file and printing a channel creation transaction:
```
[common.tools.configtxgen] main -> INFO 001 Loading configuration
[common.tools.configtxgen.localconfig] completeInitialization -> INFO 002 orderer type: etcdraft
[common.tools.configtxgen.localconfig] completeInitialization -> INFO 003 Orderer.EtcdRaft.Options unset, setting to tick_interval:"500ms" election_tick:10 heartbeat_tick:1 max_inflight_blocks:5 snapshot_interval_size:16777216
[common.tools.configtxgen.localconfig] Load -> INFO 004 Loaded configuration: /Users/fabric-samples/test-network/configtx/configtx.yaml
[common.tools.configtxgen] doOutputBlock -> INFO 005 Generating genesis block
[common.tools.configtxgen] doOutputBlock -> INFO 006 Creating application channel genesis block
[common.tools.configtxgen] doOutputBlock -> INFO 007 Writing genesis block
```

## Step two: Create the application channel

Now that we have the channel genesis block, it is easy to use the `osnadmin channel join` command to create the channel. To simplify subsequent commands, we also need to set some environment variables to establish the locations of the certificates for the nodes in the test network:

```
export ORDERER_CA=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
export ORDERER_ADMIN_TLS_SIGN_CERT=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.crt
export ORDERER_ADMIN_TLS_PRIVATE_KEY=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.key
```

Run the following command to create the channel named `channel1` on the ordering service.
```
osnadmin channel join --channelID channel1 --config-block ./channel-artifacts/channel1.block -o localhost:7053 --ca-file "$ORDERER_CA" --client-cert "$ORDERER_ADMIN_TLS_SIGN_CERT" --client-key "$ORDERER_ADMIN_TLS_PRIVATE_KEY"
```

- **`--channelID`**: Specify the name of the application channel that you provided when you created the channel genesis block.
- **`--config-block`**: Specify the location of the channel genesis block that you created with the `configtxgen` command, or the latest config block.
- **`-o`**: Specify the hostname and port of the orderer admin endpoint. For the test network ordering node this is set to `localhost:7053`.

In addition, because the `osnadmin channel` commands communicate with the ordering node using mutual TLS, you need to provide the following certificates:
- **`--ca-file`**: Specify the location and file name of the orderer organization TLS CA root certificate.
- **`--client-cert`**: Specify the location and file name of admin client signed certificate from the TLS CA.
- **`--client-key`**: Specify the location and file name of admin client private key from the TLS CA.

When successful, the output of the command contains the following:
```
Status: 201
{
	"name": "channel1",
	"url": "/participation/v1/channels/channel1",
	"consensusRelation": "consenter",
	"status": "active",
	"height": 1
}
```

The channel is active and ready for peers to join.

### Consenter vs. Follower

Notice the ordering node was joined to the channel with a `consensusRelation: "consenter"`. If you ran the command against an ordering node that is not included in the list of `Consenters:` in the `configtx.yaml` file (or the channel configuration consenter set), it is added to the channel as a `follower`. To learn more about considerations when joining additional ordering nodes see the topic on [Joining additional ordering nodes](create_channel_participation.html#step-three-join-additional-ordering-nodes).

### Active vs. onboarding

An orderer can join the channel by providing the channel **genesis block**, or the **latest config block**. If joining from the latest config block, the orderer status is set to `onboarding` until the channel ledger has caught up to the specified config block, when it becomes `active`. At this point, you could then add the orderer to the channel consenter set by submitting a channel update transaction, which will cause the  `consensusRelation` to change from `follower` to `consenter`.

## Next steps

After you have created the channel, the next steps are to join peers to the channel and deploy smart contracts. This section walks you through those processes using the test network.

### List channels on an orderer

Before you join peers to the channel, you might want to try to create additional channels. As you create more channels, the `osnadmin channel list` command is useful to view the channels that this orderer is a member of. The same parameters are used here as in the `osnadmin channel join` command from the previous step:
```
osnadmin channel list -o localhost:7053 --ca-file "$ORDERER_CA" --client-cert "$ORDERER_ADMIN_TLS_SIGN_CERT" --client-key "$ORDERER_ADMIN_TLS_PRIVATE_KEY"
```
The output of this command looks similar to:

```
Status: 200
{
	"systemChannel": null,
	"channels": [
		{
			"name": "channel1",
			"url": "/participation/v1/channels/channel1"
		}
	]
}
```

### Join peers to the channel

The test network includes two peer organizations each with one peer. But before we can use the peer CLI, we need to set some environment variables to specify which user (client MSP) we are acting as and which peer we are targeting. Set the following environment variables to indicate that we are acting as the Org1 admin and targeting the Org1 peer.

```
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

In order to use the peer CLI, we also need to modify the `FABRIC_CONFIG_PATH`:
```
export FABRIC_CFG_PATH=$PWD/../config/
```
To join the test network peer from `Org1` to the channel `channel1` simply pass the genesis block in a join request:
```
peer channel join -b ./channel-artifacts/channel1.block
```
When successful, the output of this command contains the following:
```
[channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
[channelCmd] executeJoin -> INFO 002 Successfully submitted proposal to join channel
```

We repeat these steps for the `Org2` peer. Set the following environment variables to operate the `peer` CLI as the `Org2` admin. The environment variables will also set the `Org2` peer, ``peer0.org2.example.com``, as the target peer.
```
export CORE_PEER_LOCALMSPID="Org2MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_ADDRESS=localhost:9051
```
Now repeat the command to join the peer from `Org2` to `channel1`:
```
peer channel join -b ./channel-artifacts/channel1.block
```
When successful, the output of this command contains the following:
```
[channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
[channelCmd] executeJoin -> INFO 002 Successfully submitted proposal to join channel
```

## Set anchor peer

Finally, after an organization has joined their peers to the channel, they should select **at least one** of their peers to become an anchor peer. [Anchor peers](../gossip.html#anchor-peers) are required in order to take advantage of features such as private data and service discovery. Each organization should set multiple anchor peers on a channel for redundancy. For more information about gossip and anchor peers, see the [Gossip data dissemination protocol](../gossip.html).

The endpoint information of the anchor peers of each organization is included in the channel configuration. Each channel member can specify their anchor peers by updating the channel. We will use the [configtxlator](../commands/configtxlator.html) tool to update the channel configuration and select an anchor peer for `Org1` and `Org2`.

**Note:** If [jq](https://stedolan.github.io/jq/) is not already installed on your local machine, you need to install it now to complete these steps.  

We will start by selecting the peer from `Org1` to be an anchor peer. The first step is to pull the most recent channel configuration block using the `peer channel fetch` command. Set the following environment variables to operate the `peer` CLI as the `Org1` admin:
```
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

You can use the following command to fetch the channel configuration:
```
peer channel fetch config channel-artifacts/config_block.pb -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com -c channel1 --tls --cafile "$ORDERER_CA"
```
Because the most recent channel configuration block is the channel genesis block, the command returns block `0` from the channel.
```
[channelCmd] InitCmdFactory -> INFO 001 Endorser and orderer connections initialized
[cli.common] readBlock -> INFO 002 Received block: 0
[channelCmd] fetch -> INFO 003 Retrieving last config block: 0
[cli.common] readBlock -> INFO 004 Received block: 0
```

The channel configuration block `config_block.pb` is stored in the `channel-artifacts` folder to keep the update process separate from other artifacts. Change into the `channel-artifacts` folder to complete the next steps:
```
cd channel-artifacts
```
We can now start using the `configtxlator` tool to start working with the channel configuration. The first step is to decode the block from protobuf into a JSON object that can be read and edited. We also strip away the unnecessary block data, leaving only the channel configuration.

```
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
jq '.data.data[0].payload.data.config' config_block.json > config.json
```

These commands convert the channel configuration block into a streamlined JSON, `config.json`, that will serve as the baseline for our update. Because we don't want to edit this file directly, we will make a copy that we can edit. We will use the original channel config in a future step.
```
cp config.json config_copy.json
```

You can use the `jq` tool to add the `Org1` anchor peer to the channel configuration.
```
jq '.channel_group.groups.Application.groups.Org1MSP.values += {"AnchorPeers":{"mod_policy": "Admins","value":{"anchor_peers": [{"host": "peer0.org1.example.com","port": 7051}]},"version": "0"}}' config_copy.json > modified_config.json
```

After this step, we have an updated version of channel configuration in JSON format in the `modified_config.json` file. We can now convert both the original and modified channel configurations back into protobuf format and calculate the difference between them.
```
configtxlator proto_encode --input config.json --type common.Config --output config.pb
configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
configtxlator compute_update --channel_id channel1 --original config.pb --updated modified_config.pb --output config_update.pb
```

The new protobuf named `config_update.pb` contains the anchor peer update that we need to apply to the channel configuration. We can wrap the configuration update in a transaction envelope to create the channel configuration update transaction.

```
configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json
echo '{"payload":{"header":{"channel_header":{"channel_id":"channel1", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json
configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb
```

We can now use the final artifact, `config_update_in_envelope.pb`, that can be used to update the channel. Navigate back to the `test-network` directory:
```
cd ..
```

We can add the anchor peer by providing the new channel configuration to the `peer channel update` command. Because we are updating a section of the channel configuration that only affects `Org1`, other channel members do not need to approve the channel update.
```
peer channel update -f channel-artifacts/config_update_in_envelope.pb -c channel1 -o localhost:7050  --ordererTLSHostnameOverride orderer.example.com --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
```

When the channel update is successful, you should see the following response:
```
[channelCmd] update -> INFO 002 Successfully submitted channel update
```

We can also set the peer from `Org2` to be an anchor peer. Because we are going through the process a second time, we will go through the steps more quickly. Set the environment variables to operate the `peer` CLI as the `Org2` admin:
```
export CORE_PEER_LOCALMSPID="Org2MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_ADDRESS=localhost:9051
```

Pull the latest channel configuration block, which is now the second block on the channel:
```
peer channel fetch config channel-artifacts/config_block.pb -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com -c channel1 --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
```

Navigate back to the `channel-artifacts` directory:
```
cd channel-artifacts
```

You can then decode and copy the configuration block.
```
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
jq '.data.data[0].payload.data.config' config_block.json > config.json
cp config.json config_copy.json
```

Add the `Org2` peer that is joined to the channel as the anchor peer in the channel configuration:
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

Update the channel and set the `Org2` anchor peer by issuing the following command:
```
peer channel update -f channel-artifacts/config_update_in_envelope.pb -c channel1 -o localhost:7050  --ordererTLSHostnameOverride orderer.example.com --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
```

If you want to learn more about how to submit a channel update request, see [update a channel configuration](../config_update.html).

You can confirm that the channel has been updated successfully by running the `peer channel info` command:
```
peer channel getinfo -c channel1
```
Now that the channel has been updated by adding two channel configuration blocks to the channel genesis block, the height of the channel will have grown to three and the hashes are updated:
```
Blockchain info: {"height":3,"currentBlockHash":"GKqqk/HNi9x/6YPnaIUpMBlb0Ew6ovUnSB5MEF7Y5Pc=","previousBlockHash":"cl4TOQpZ30+d17OF5YOkX/mtMjJpUXiJmtw8+sON8a8="}
```

## Deploy a chaincode to the new channel

We can confirm that the channel was created successfully by deploying a chaincode to the channel. We can use the `network.sh` script to deploy the Basic asset transfer chaincode to any test network channel. Deploy a chaincode to our new channel using the following command:
```
./network.sh deployCC -ccn basic -ccp ../asset-transfer-basic/chaincode-go/ -ccl go -c channel1
```

After you run the command, you should see the chaincode being deployed to the channel in your logs.

```
Committed chaincode definition for chaincode 'basic' on channel 'channel1':
Version: 1.0, Sequence: 1, Endorsement Plugin: escc, Validation Plugin: vscc, Approvals: [Org1MSP: true, Org2MSP: true]
Query chaincode definition successful on peer0.org2 on channel 'channel1'
Chaincode initialization is not required
```
Then run the following command to initialize some assets on the ledger:
```
peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C channel1 -n basic --peerAddresses localhost:7051 --tlsRootCertFiles "${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt" --peerAddresses localhost:9051 --tlsRootCertFiles "${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt" -c '{"function":"InitLedger","Args":[]}'
```
When successful you will see:
```
[chaincodeCmd] chaincodeInvokeOrQuery -> INFO 001 Chaincode invoke successful. result: status:200
```

Confirm the assets were added to the ledger by issuing the following query:

```
peer chaincode query -C channel1 -n basic -c '{"Args":["getAllAssets"]}'
```

You should see output similar to the following:
```
[{"ID":"asset1","color":"blue","size":5,"owner":"Tomoko","appraisedValue":300},
{"ID":"asset2","color":"red","size":5,"owner":"Brad","appraisedValue":400},
{"ID":"asset3","color":"green","size":10,"owner":"Jin Soo","appraisedValue":500},
{"ID":"asset4","color":"yellow","size":10,"owner":"Max","appraisedValue":600},
{"ID":"asset5","color":"black","size":15,"owner":"Adriana","appraisedValue":700},
{"ID":"asset6","color":"white","size":15,"owner":"Michel","appraisedValue":800}]
```

### Create a channel without the test network

This tutorial has taken you through the basic steps to create a channel on the test network by using the `osnadmin channel join` command. When you are ready to build your own network, follow the steps in the [Create a channel](create_channel_participation.html) tutorial to learn more about using the `osnadmin channel` commands.
<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
