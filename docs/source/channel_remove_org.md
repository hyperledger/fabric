# Removing an Org from a Channel

**_NOTE:_**

As a prerequisite of this tutorial, a blockchain network consisting of 3 organizations should have been created by referencing steps in [Adding an Org to a Channel](./channel_update_tutorial.html).

---

This tutorial removes Org2 from the Hyperledger Fabric test network consisting of Org1, Org2 and Org3.

## Stop Org2 Peer

Before removing Org2 from the Fabric network, stop the Org2 peer.

```bash
docker stop peer0.org2.example.com
```

## Fetch the Configuration

We will be operating from the root of the `test-network` subdirectory within your local clone of `fabric-samples`.

```bash
cd fabric-samples/test-network
```

Issue the following commands to operate as the Org1 admin.

```bash
# you can issue all of these commands at once

export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=${PWD}/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=Org1MSP
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

We can now issue the command to fetch the latest config block:

```bash
peer channel fetch config channel-artifacts/config_block.pb -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com -c channel1 --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
```

This command saves the binary protobuf channel configuration block to `config_block.pb`. Note that the choice of name and file extension is arbitrary. However, following a convention which identifies both the type of object being represented and its encoding (protobuf or JSON) is recommended.

When you issued the `peer channel fetch` command, the following output is displayed in your logs:

```
2024-03-18 14:35:27.837 CST 0001 INFO [channelCmd] InitCmdFactory -> Endorser and orderer connections initialized
2024-03-18 14:35:27.845 CST 0002 INFO [cli.common] readBlock -> Received block: 4
2024-03-18 14:35:27.845 CST 0003 INFO [channelCmd] fetch -> Retrieving last config block: 4
2024-03-18 14:35:27.848 CST 0004 INFO [cli.common] readBlock -> Received block: 4
```

## Convert the Configuration to JSON and Trim It Down

The channel configuration block was stored in the `channel-artifacts` folder to keep the update process separate from other artifacts. Change into the `channel-artifacts` folder to complete the next steps:

```bash
cd channel-artifacts
```

Now we will make use of the `configtxlator` tool to decode this channel configuration block into JSON format (which can be read and modified by humans):

```bash
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
```

We also must strip away all of the headers, metadata, creator signatures, and so on that are irrelevant to the change we want to make. We accomplish this by means of the `jq` tool (you will need to install the jq tool on your local machine):

```bash
jq '.data.data[0].payload.data.config' config_block.json >config.json
```

This command leaves us with a trimmed down JSON object – `config.json` – which will serve as the baseline for our config update.

## Remove Org2 from Channel

We’ll use the `jq` tool once more to remove `Org2MSP` from the channel's application group field:

```bash
jq 'del(.channel_group.groups.Application.groups.Org2MSP)' config.json > modified_config.json
```

## Remove the Org2 Crypto Material from Config Block

Now we have two JSON files of interest – `config.json` and `modified_config.json`. The initial file contains all three Orgs, whereas the “modified” file contains only Org1 and Org3 material. At this point it’s simply a matter of re-encoding these two JSON files and calculating the delta.

First, translate `config.json` back into a protobuf called `config.pb`:

```bash
configtxlator proto_encode --input config.json --type common.Config --output config.pb
```

Next, encode `modified_config.json` to `modified_config.pb`:

```bash
configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
```

Now use `configtxlator` to calculate the delta between these two config protobufs. This command will output a new protobuf binary named `config_update.pb`:

```bash
configtxlator compute_update --channel_id channel1 --original config.pb --updated modified_config.pb --output config_update.pb
```

Decode delta object into JSON format:

```bash
configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json
```

Now, we have a decoded update file – `config_update.json` – that we need to wrap in an envelope message. This step will give us back the header field that we stripped away earlier. We’ll name this file `config_update_in_envelope.json`:

```bash
echo '{"payload":{"header":{"channel_header":{"channel_id":"channel1", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . > config_update_in_envelope.json
```

Using our properly formed JSON – `config_update_in_envelope.json` – we will leverage the `configtxlator` tool one last time and convert it into the fully fledged protobuf format that Fabric requires. We’ll name our final update object `config_update_in_envelope.pb`:

```bash
configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output config_update_in_envelope.pb
```

## Sign and Submit the Config Update

Before writing the configuration to the ledger, we need signatures from the requisite Admin users.

First, let’s sign this update proto as Org1. Navigate back to the `test-network` directory:

```bash
cd ..
```

Export the Org1 environment variables:

```bash
export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=${PWD}/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=Org1MSP
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

Run the following `peer channel signconfigtx` command to sign the update as Org1:

```bash
peer channel signconfigtx -f channel-artifacts/config_update_in_envelope.pb
```

Export the Org2 environment variables:

```bash
export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=${PWD}/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=Org2MSP
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_ADDRESS=localhost:9051
```

Run the following `peer channel signconfigtx` command to sign the update as Org2:

```bash
peer channel signconfigtx -f channel-artifacts/config_update_in_envelope.pb
```

Export the Org3 environment variables:

```bash
export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=${PWD}/../config/
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=Org3MSP
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp
export CORE_PEER_ADDRESS=localhost:11051
```

Run the following `peer channel signconfigtx` command to sign the update as Org3:

```bash
peer channel signconfigtx -f channel-artifacts/config_update_in_envelope.pb
```

We will now finally issue the `peer channel update` command to send the update to channel:

```bash
peer channel update -f channel-artifacts/config_update_in_envelope.pb -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com -c channel1 --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
```

You should see a message similar to the following if your update has been submitted successfully:

```bash
2024-03-18 14:40:10.761 CST 0002 INFO [channelCmd] update -> Successfully submitted channel update
```

Now, all operations to remove Org2 from the Fabric environment are complete.
You can inspect the logs for peer0.org1.example.com and peer0.org3.example.com by issuing the following command:

```
docker logs peer0.org1.example.com -f
docker logs peer0.org3.example.com -f
```

Inspect the logs for the peers, you will see the following gossip warning message about peer0.org2.example.com. Since Org2 is already removed from the Fabric network, Org1 peer and Org3 peer are not able to connect to Org2 peer any more.

```
2024-03-18 06:40:10.811 UTC 0086 WARN [gossip.gossip] func3 -> Failed determining organization of 87333f25423a5270cddbed5078fde14056ed2899e4ce7ffc6476e81f330201d1
2024-03-18 06:40:11.108 UTC 0087 WARN [peer.gossip.sa] OrgByPeerIdentity -> Peer Identity [7b 22 43 4e 22 3a 22 70 65 65 72 30 2e 6f 72 67 32 2e 65 78 61 6d 70 6c 65 2e 63 6f 6d 22 2c 22 49 73 73 75 65 72 2d 43 4e 22 3a 22 63 61 2e 6f 72 67 32 2e 65 78 61 6d 70 6c 65 2e 63 6f 6d 22 2c 22 49 73 73 75 65 72 2d 4c 2d 53 54 2d 43 22 3a 22 5b 53 61 6e 20 46 72 61 6e 63 69 73 63 6f 5d 2d 5b 5d 2d 5b 55 53 5d 22 2c 22 49 73 73 75 65 72 2d 4f 55 22 3a 6e 75 6c 6c 2c 22 4c 2d 53 54 2d 43 22 3a 22 5b 53 61 6e 20 46 72 61 6e 63 69 73 63 6f 5d 2d 5b 5d 2d 5b 55 53 5d 22 2c 22 4d 53 50 22 3a 22 4f 72 67 32 4d 53 50 22 2c 22 4f 55 22 3a 5b 22 70 65 65 72 22 5d 7d] cannot be desirialized. No MSP found able to do that.

```

## Restart Org1 Peer and Org3 Peer

You can eliminate the warnings by restarting Org1 peer and Org3 peer.

```bash
docker stop peer0.org1.example.com
docker start peer0.org1.example.com
docker stop peer0.org3.example.com
docker start peer0.org3.example.com
```

## Updating Collection Definition File (Optional)

In case you use private data collection in your chaincode, you should now remove Org2MSP from the collection definition and update the chaincode since Org2MSP is no longer defined in the Fabric network.
For example, a collection is defined to hold private data between Org1 and Org2:

```json
[
  {
    "name": "pdc1",
    "policy": "OR('Org1MSP.member', 'Org2MSP.member')",
    "requiredPeerCount": 0,
    "maxPeerCount": 3,
    "blockToLive": 1000000,
    "memberOnlyRead": true,
    "memberOnlyWrite": false,
    "endorsementPolicy": { "signaturePolicy": "OR('Org1MSP.member', 'Org2MSP.member')" },
  },
]
```

After Org2 is removed, reference to Org2MSP should be removed from collection definition, so the collection is now held by Org1 only:

```json
[
  {
    "name": "pdc1",
    "policy": "OR('Org1MSP.member')",
    "requiredPeerCount": 0,
    "maxPeerCount": 3,
    "blockToLive": 1000000,
    "memberOnlyRead": true,
    "memberOnlyWrite": false,
    "endorsementPolicy": { "signaturePolicy": "OR('Org1MSP.member')" },
  },
]
```

## Transfer Collection to Another Org (Optional)

What should we do if an existing collection is held by the removed Org only.
For example, the following collection pdc2 is held by Org2 only:

```json
[
  {
    "name": "pdc2",
    "policy": "OR('Org2MSP.member')",
    "requiredPeerCount": 0,
    "maxPeerCount": 3,
    "blockToLive": 1000000,
    "memberOnlyRead": true,
    "memberOnlyWrite": false,
    "endorsementPolicy": { "signaturePolicy": "OR('Org2MSP.member')" },
  },
]
```

As stated in [Updating a collection definition](./private-data-arch.html#updating-a-collection-definition), existing collections must be included in collection definition file, so we cannot simply remove the collection from collection definition file.

> If a collection configuration is specified when updating the chaincode definition, a definition for each of the existing collections must be included.

As a workaround, you could transfer the collection to another existing organization. In this case, we transferred pdc2 to Org3:

```json
[
  {
    "name": "pdc2",
    "policy": "OR('Org3MSP.member')",
    "requiredPeerCount": 0,
    "maxPeerCount": 3,
    "blockToLive": 1000000,
    "memberOnlyRead": true,
    "memberOnlyWrite": false,
    "endorsementPolicy": { "signaturePolicy": "OR('Org3MSP.member')" },
  },
]
```

After this transfer, Org3 peer will try to fetch private data of pdc2 from other peers. This is called private data reconciliation. Private data reconciliation occurs periodically based on `peer.gossip.pvtData.reconciliationEnabled` and `peer.gossip.pvtData.reconcileSleepInterval` properties in core.yaml.
Since Org2 peer is already removed from Fabric network, Org3 peer will have no way to retrieve private data from Org2 peer. The following warning message will display in Org3 peer log:

```
2024-03-18 06:15:51.065 UTC 0047 WARN [gossip.privdata] fetchPrivateData -> Do not know any peer in the channel (channel1) that matches the policies , aborting channel=channel1
2024-03-18 06:15:51.065 UTC 0048 ERRO [gossip.privdata] reconcile -> reconciliation error when trying to fetch missing items from different peers: Empty membership channel=channel1
2024-03-18 06:15:51.065 UTC 0049 ERRO [gossip.privdata] run -> Failed to reconcile missing private info, error:  Empty membership channel=channel1
```

You could configure `peer.gossip.pvtData.reconciliationEnabled=false` for Org3 peer to stop reconciliation and suppress the error log.
