# Adding Orderer To An Existing Network

## Create an initial cluster
Fabric supports adding new orderer to an existing functioning network.
We will lay out a simple scenario of such functionality using the **test-network** sample (from [fabric samples repo](https://github.com/hyperledger/fabric-samples)).

### Extending the test network to support the fifth orderer
We extend the `docker-compose-bft` and the `crypto-config-orderer.yaml` to support 5 orderers.\
In the `crypto-config-orderer.yaml` we should add:
```yaml
- Hostname: orderer5
  SANS:
    - localhost
```

In the `docker-compose-bft` we should create a new volume in the volumes section:
```yaml
volumes:
  ...
  orderer5.example.com
```
Now, add the definition of the new orderer:
(Note that you can change the ports according to your needs)
```yaml
  orderer5.example.com:
    container_name: orderer5.example.com
    image: hyperledger/fabric-orderer:latest
    labels:
      service: hyperledger-fabric
    environment:
      - FABRIC_LOGGING_SPEC=DEBUG
      - ORDERER_GENERAL_LISTENADDRESS=0.0.0.0
      - ORDERER_GENERAL_LISTENPORT=7060
      - ORDERER_GENERAL_LOCALMSPID=OrdererMSP
      - ORDERER_GENERAL_LOCALMSPDIR=/var/hyperledger/orderer/msp
      # enabled TLS
      - ORDERER_GENERAL_TLS_ENABLED=true
      - ORDERER_GENERAL_TLS_PRIVATEKEY=/var/hyperledger/orderer/tls/server.key
      - ORDERER_GENERAL_TLS_CERTIFICATE=/var/hyperledger/orderer/tls/server.crt
      - ORDERER_GENERAL_TLS_ROOTCAS=[/var/hyperledger/orderer/tls/ca.crt]
      - ORDERER_GENERAL_CLUSTER_CLIENTCERTIFICATE=/var/hyperledger/orderer/tls/server.crt
      - ORDERER_GENERAL_CLUSTER_CLIENTPRIVATEKEY=/var/hyperledger/orderer/tls/server.key
      - ORDERER_GENERAL_CLUSTER_ROOTCAS=[/var/hyperledger/orderer/tls/ca.crt]
      - ORDERER_GENERAL_BOOTSTRAPMETHOD=none
      - ORDERER_CHANNELPARTICIPATION_ENABLED=true
      - ORDERER_ADMIN_TLS_ENABLED=true
      - ORDERER_ADMIN_TLS_CERTIFICATE=/var/hyperledger/orderer/tls/server.crt
      - ORDERER_ADMIN_TLS_PRIVATEKEY=/var/hyperledger/orderer/tls/server.key
      - ORDERER_ADMIN_TLS_ROOTCAS=[/var/hyperledger/orderer/tls/ca.crt]
      - ORDERER_ADMIN_TLS_CLIENTROOTCAS=[/var/hyperledger/orderer/tls/ca.crt]
      - ORDERER_ADMIN_LISTENADDRESS=0.0.0.0:7061
      - ORDERER_OPERATIONS_LISTENADDRESS=orderer5.example.com:9450
      - ORDERER_METRICS_PROVIDER=prometheus
    working_dir: /root
    command: orderer
    volumes:
      - ../organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp:/var/hyperledger/orderer/msp
      - ../organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/tls:/var/hyperledger/orderer/tls
      - orderer5.example.com:/var/hyperledger/production/orderer
    ports:
      - 7060:7060
      - 7061:7061
      - 9450:9450
    networks:
      - test
```

We also add the following volume to the CLI container definition:
```yaml
volumes:
  - ../organizations/ordererOrganizations/example.com/users/Admin@example.com/msp:/var/hyperledger/orderer/msp
  - ../organizations/ordererOrganizations/example.com/users/Admin@example.com/tls:/var/hyperledger/orderer/tls
```

### Running the cluster
Use:
```shell
./network.sh createChannel -bft
```
This command will start a network of 4 orderers and 2 peers and 1 CLI, a container of the fifth orderer will be started
as well, but is not a part of the network at this stage. This command also will create a channel named "mychannel" in which the 4 orderers and the 2 peers participate.

## Altering the config
The following commands should be executing from the CLI container.

### Getting the last config block
The `peer` command uses environment variables to define the context of the organization in which it will run, we will
change the context to:
```shell
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=$PEER0_ORG1_CA
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051

export ORDERER_CA=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```
Now, in order to get the last config block we will make:
```shell
peer channel fetch config config_block.pb -o orderer.example.com:7050 --ordererTLSHostnameOverride orderer.example.com -c mychannel --tls --cafile "$ORDERER_CA"
```

### Convert the block to a JSON
Convert the block to JSON:
```shell
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
```
Extract the config from the JSON block:
```shell
jq .data.data[0].payload.data.config config_block.json > original_config.json
```

### Add the fifth orderer to the config
The output of this stage is an update TX, you can calculate the TX from the CLI container,
or copy the `original_config.json` and make all the changes on your local machine.\
Create a copy of `original_config.json` named `modified_config.json`.
In the new JSON file we need to make 4 changes:

#### 1. Add the orderer to the known endpoints
Go to **channel_group &rarr; groups &rarr; Orderer &rarr; groups &rarr; OrdererOrg &rarr; values &rarr; Endpoints &rarr; value &rarr; addresses**
and add the new orderer endpoint.
```json lines
[
    "orderer.example.com:7050",
    "orderer2.example.com:7052",
    "orderer3.example.com:7056",
    "orderer4.example.com:7058",
    "orderer5.example.com:7060"
]
```

#### 2. Add the orderer to the known identities
Go to **channel_group &rarr; groups &rarr; Orderer &rarr; policies &rarr; BlockValidation &rarr; policy &rarr; value &rarr; identities**
and add the base64 encode of the identity certificate, please correct the path according to your needs.

```json
{
    "principal": {
          "id_bytes": ".../test-network/organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/signcerts/orderer5.example.com-cert.pem",
          "mspid": "OrdererMSP"
    },
    "principal_classification": "IDENTITY"
}
```

#### 3. Add the orderer to the policy rules
Go to **channel_group &rarr; groups &rarr; Orderer &rarr; policies &rarr; BlockValidation &rarr; policy &rarr; value &rarr; rule**,
change the **n** to be:
```
# Given that the new number of nodes in cluster is num_of_nodes:
f = int((num_of_nodes - 1) / 3)
n = ceil((num_of_nodes + f + 1) / 2)
```

And add a `signed_by` object for the new orderer:

```json
{
  "n_out_of": {
    "n": 4,
    "rules": [
      {
        "signed_by": 0
      },
      {
        "signed_by": 1
      },
      {
        "signed_by": 2
      },
      {
        "signed_by": 3
      },
      {
        "signed_by": 4
      }
    ]
  }
}
```

#### 4. Add the orderer to the concenter mapping
Go to **channel_group &rarr; groups &rarr; Orderer &rarr; values &rarr; Orderers &rarr; value &rarr; consenter_mapping**
and add the base64 encode of the identity, client TLS and server TLS certificates, please correct the paths according
to your needs.
```json
{
    "client_tls_cert": ".../test-network/organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/tlscacerts/tlsca.example.com-cert.pem",
    "host": "orderer5.example.com",
    "id": 5,
    "identity": ".../test-network/organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/signcerts/orderer5.example.com-cert.pem",
    "msp_id": "OrdererMSP",
    "port": 7060,
    "server_tls_cert": ".../test-network/organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"
}
```

We made this process easy and created a Python script which can be found in the `scripts`
[subfolder](https://github.com/hyperledger/fabric-samples/tree/main/test-network/scripts)
that does just that (steps 1-4)!
Example for the script usage:
```shell
python3 scripts/add_new_orderer_to_config.py original_config.json modified_config.json \
-a orderer5.example.com:7060 \
-i ./organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/signcerts/orderer5.example.com-cert.pem \
-s ./organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/tlscacerts/tlsca.example.com-cert.pem \
-c ./organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
```

Calculate the update using:
```shell
configtxlator proto_encode --input original_config.json --type common.Config --output original_config.pb
configtxlator proto_encode --input modified_config.json --type common.Config --output modified_config.pb
configtxlator compute_update --channel_id mychannel --original original_config.pb --updated modified_config.pb --output config_update.pb
configtxlator proto_decode --input config_update.pb --type common.ConfigUpdate --output config_update.json
echo '{"payload":{"header":{"channel_header":{"channel_id":"mychannel", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . >config_update_in_envelope.json
configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output envelope.pb
```

`envelope.pb` is the config update TX, note that it does not contain any paths,
if it was created on your local machine, please copy it to the CLI container.

## Make the update
From the CLI we need to sign the TX using one of the peers' organizations and the orderers' organization.
Since we are in the context of the peer organization `Org1`, we can simply:
```shell
peer channel signconfigtx -f envelope.pb
```
Now we switch to the orderer organization `Orderer`:
```shell
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="OrdererMSP"
export CORE_PEER_TLS_ROOTCERT_FILE=/var/hyperledger/orderer/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=/var/hyperledger/orderer/msp
export CORE_PEER_ADDRESS=localhost:7050
```
And we update the orderer:
```shell
peer channel update -o orderer.example.com:7050 --ordererTLSHostnameOverride orderer.example.com -c mychannel -f envelope.pb --tls --cafile "$ORDERER_CA"
```

The output of this command looks similar to:
```
INFO [channelCmd] InitCmdFactory -> Endorser and orderer connections initialized
INFO [channelCmd] update -> Successfully submitted channel update
```

The new orderer has been added to the cluster, but not to the test channel.

## Use the `osnadmin` CLI to add the new orderer to the test channel

The new orderer needs to run the following command:

```shell
export OSN_TLS_CA_ROOT_CERT=${PWD}/organizations/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem
export ADMIN_TLS_SIGN_CERT=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/tls/server.crt
export ADMIN_TLS_PRIVATE_KEY=${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer5.example.com/tls/server.key

osnadmin channel join --channelID [CHANNEL_NAME]  --config-block [CHANNEL_CONFIG_BLOCK] -o [ORDERER_ADMIN_LISTENADDRESS] --ca-file $OSN_TLS_CA_ROOT_CERT --client-cert $ADMIN_TLS_SIGN_CERT --client-key $ADMIN_TLS_PRIVATE_KEY
```

Replace:
- `CHANNEL_NAME` with the name you want to call this channel.
- `CHANNEL_CONFIG_BLOCK` with the path and file name of the genesis block or the latest config block.
- `ORDERER_ADMIN_LISTENADDRESS` corresponds to the `Orderer.Admin.ListenAddress` defined in the `orderer.yaml` for this orderer.
- `OSN_TLS_CA_ROOT_CERT` with the path and file name of the orderer organization TLS CA root certificate and intermediate certificate if using an intermediate TLS CA.
- `ADMIN_TLS_SIGN_CERT` with the path and file name of the admin client signed certificate from the TLS CA.
- `ADMIN_TLS_PRIVATE_KEY` with the path and file name of the admin client private key from the TLS CA.

For example:
```shell
osnadmin channel join --channelID mychannel --config-block ./channel-artifacts/mychannel.block -o localhost:7061 --ca-file "$OSN_TLS_CA_ROOT_CERT" --client-cert "$ADMIN_TLS_SIGN_CERT" --client-key "$ADMIN_TLS_PRIVATE_KEY"
```

**Note:** Because the connection between the `osnadmin` CLI and the orderer requires mutual TLS, you need to pass the `--client-cert` and `--client-key` parameters on each `osadmin` command. The `--client-cert` parameter points to the admin client certificate and `--client-key` refers to the admin client private key, both issued by the admin client TLS CA.

The output of this command looks similar to:
```
Status: 201
{
    "name": "mychannel",
    "url": "/participation/v1/channels/mychannel",
    "consensusRelation": "follower",
    "status": "onboarding",
    "height": 0
}
```

You should see something similar to the following in the new orderer logs:
```
DEBU [orderer.consensus.smartbft.consensus] ProcessMessages -> 5 got message from 1: <HeartBeat with view: 0, seq: 7 channel=mychannel
DEBU [orderer.consensus.smartbft.consensus] handleHeartBeat -> Received heartbeat from 1, last heartbeat was 5.995614586s ago channel=mychannel
DEBU [orderer.consensus.smartbft.consensus] followerTick -> Last heartbeat from 1 was 1.000437542s ago channel=mychannel
DEBU [orderer.consensus.smartbft.consensus] followerTick -> Last heartbeat from 1 was 2.003549876s ago channel=mychannel
DEBU [orderer.consensus.smartbft.consensus] followerTick -> Last heartbeat from 1 was 3.00110746s ago channel=mychannel
DEBU [orderer.consensus.smartbft.consensus] followerTick -> Last heartbeat from 1 was 3.99966021s ago channel=mychannel
DEBU [orderer.consensus.smartbft.consensus] followerTick -> Last heartbeat from 1 was 5.000054669s ago channel=mychannel
DEBU [orderer.consensus.smartbft.consensus] followerTick -> Last heartbeat from 1 was 5.999811586s ago channel=mychannel
```

You can use the following command to confirm that the status of the added orderer:
```shell
osnadmin channel list --channelID mychannel -o localhost:7061 --ca-file "$OSN_TLS_CA_ROOT_CERT" --client-cert "$ADMIN_TLS_SIGN_CERT" --client-key "$ADMIN_TLS_PRIVATE_KEY"
```

You should see something similar to the following output that the consensusRelation status for the added orderer automatically changes to consenter:
```shell
{
        "name": "mychannel",
        "url": "/participation/v1/channels/mychannel",
        "consensusRelation": "consenter",
        "status": "active",
        "height": 4
}
```

You can read further about the osnadmin command [here](https://hyperledger-fabric.readthedocs.io/en/latest/create_channel/create_channel_participation.html#step-two-use-the-osnadmin-cli-to-add-the-first-orderer-to-the-channel).

# Removing an orderer from an existing network

Switch to the peer organization:
```shell
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=Org1MSP
export CORE_PEER_TLS_ROOTCERT_FILE=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org1.example.com/tlsca/tlsca.org1.example.com-cert.pem
export CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051
```

Fetch the last block using:
```shell
peer channel fetch config config_block.pb -o orderer.example.com:7050 --ordererTLSHostnameOverride orderer.example.com -c mychannel --tls --cafile "$ORDERER_CA"
```

Convert it to a JSON:
```shell
configtxlator proto_decode --input config_block.pb --type common.Block --output config_block.json
jq .data.data[0].payload.data.config config_block.json > original_config.json
```

Now remove the new orderer from the JSON (in reverse order to what was done in the previous relevant section)
and save it as `modified_config.json`.

Now compute the update:
```shell
echo '{"payload":{"header":{"channel_header":{"channel_id":"mychannel", "type":2}},"data":{"config_update":'$(cat config_update.json)'}}}' | jq . >config_update_in_envelope.json
configtxlator proto_encode --input config_update_in_envelope.json --type common.Envelope --output envelope.pb
```

Sign it using the peer organization:
```shell
peer channel signconfigtx -f envelope.pb
```

Now switch to the orderer organization and post it:
```shell
export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="OrdererMSP"
export CORE_PEER_TLS_ROOTCERT_FILE=/var/hyperledger/orderer/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=/var/hyperledger/orderer/msp
export CORE_PEER_ADDRESS=localhost:7050
peer channel update -o orderer.example.com:7050 --ordererTLSHostnameOverride orderer.example.com -c mychannel -f envelope.pb --tls --cafile "$ORDERER_CA"
```

The result should be:
```
INFO [channelCmd] InitCmdFactory -> Endorser and orderer connections initialized
INFO [channelCmd] update -> Successfully submitted channel update
```

Now let us remove the orderer from the channel by using the command:
```shell
osnadmin channel remove -o localhost:7061 --ca-file "$ORDERER_CA" --client-cert "$ORDERER_ADMIN_TLS_SIGN_CERT" --client-key "$ORDERER_ADMIN_TLS_PRIVATE_KEY" --channelID mychannel
```

The result should be:
```
Status: 204
```

You should see something similar to the following in your orderer logs:
```
INFO [orderer.consensus.smartbft.chain] Halt -> Shutting down chain channel=mychannel
INFO [orderer.consensus.smartbft.consensus] func1 -> Exiting channel=mychannel
INFO [orderer.consensus.smartbft.consensus] func1 -> Exiting channel=mychannel
INFO [orderer.common.multichannel] removeMember -> Removed channel: mychannel
```