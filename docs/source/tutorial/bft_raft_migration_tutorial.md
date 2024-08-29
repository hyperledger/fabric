# Operational Runbook: Migrating from Raft to BFT in Hyperledger Fabric

## Prerequisites

- A running Fabric network with Raft consensus
- Administrative access to the ordering service
- Fabric version 3.0.0 or higher on all nodes
- Backup of the current system

## Step 1: Prepare the Environment

```bash
export FABRIC_CFG_PATH=$PWD/config
export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
export CHANNEL_NAME=mychannel
```

## Step 2: Verify Current Consensus Type and Consenter Mapping

```bash
peer channel fetch config config_block.pb -o orderer.example.com:7050 -c $CHANNEL_NAME --tls --cafile $ORDERER_CA

configtxlator proto_decode --input config_block.pb --type common.Block | jq .data.data[0].payload.data.config > current_config.json

# Check current consensus type
jq '.channel_group.groups.Orderer.values.ConsensusType.value.type' current_config.json

# Check current consenter mapping
jq '.channel_group.groups.Orderer.values.Orderers.value.consenter_mapping' current_config.json
```

## Step 3: Enter Maintenance Mode

```bash
cp current_config.json modified_config.json

jq '.channel_group.groups.Orderer.values.ConsensusType.value.state = "STATE_MAINTENANCE"' modified_config.json > updated_config.json

configtxlator proto_encode --input current_config.json --type common.Config --output current_config.pb
configtxlator proto_encode --input updated_config.json --type common.Config --output updated_config.pb
configtxlator compute_update --channel_id $CHANNEL_NAME --original current_config.pb --updated updated_config.pb --output config_update.pb

peer channel signconfigtx -f config_update.pb
peer channel update -f config_update.pb -c $CHANNEL_NAME -o orderer.example.com:7050 --tls --cafile $ORDERER_CA
```

## Step 4: Backup the System

```bash
docker stop $(docker ps -q --filter name=orderer)
tar -czf orderer_backup.tar.gz /var/hyperledger/production/orderer
docker start $(docker ps -aq --filter name=orderer)
```

## Step 5: Switch to BFT and Update Consenter Mapping

# Prepare the new consenter mapping
cat << EOF > consenter_mapping.json
[
  {
    "id": 1,
    "host": "orderer1.example.com",
    "port": 7050,
    "msp_id": "OrdererMSP",
    "identity": "base64_encoded_identity",
    "client_tls_cert": "base64_encoded_client_cert",
    "server_tls_cert": "base64_encoded_server_cert"
  },
  {
    "id": 2,
    "host": "orderer2.example.com",
    "port": 7050,
    "msp_id": "OrdererMSP",
    "identity": "base64_encoded_identity",
    "client_tls_cert": "base64_encoded_client_cert",
    "server_tls_cert": "base64_encoded_server_cert"
  },
  {
    "id": 3,
    "host": "orderer3.example.com",
    "port": 7050,
    "msp_id": "OrdererMSP",
    "identity": "base64_encoded_identity",
    "client_tls_cert": "base64_encoded_client_cert",
    "server_tls_cert": "base64_encoded_server_cert"
  },
  {
    "id": 4,
    "host": "orderer4.example.com",
    "port": 7050,
    "msp_id": "OrdererMSP",
    "identity": "base64_encoded_identity",
    "client_tls_cert": "base64_encoded_client_cert",
    "server_tls_cert": "base64_encoded_server_cert"
  }
]
EOF

# Update the config with new consensus type, metadata, and consenter mapping
jq --slurpfile mapping consenter_mapping.json '
  .channel_group.groups.Orderer.values.ConsensusType.value.type = "BFT" |
  .channel_group.groups.Orderer.values.ConsensusType.value.metadata = {
    "options": {
      "RequestBatchMaxInterval": "200ms",
      "RequestForwardTimeout": "5s",
      "RequestComplainTimeout": "20s",
      "RequestAutoRemoveTimeout": "3m0s",
      "ViewChangeResendInterval": "5s",
      "ViewChangeTimeout": "20s",
      "LeaderHeartbeatTimeout": "1m0s",
      "CollectTimeout": "1s",
      "IncomingMessageBufferSize": 200,
      "RequestPoolSize": 100000,
      "LeaderHeartbeatCount": 10
    }
  } |
  .channel_group.groups.Orderer.values.Orderers.value.consenter_mapping = $mapping[0]
' updated_config.json > bft_config.json

# Create and submit the config update
configtxlator proto_encode --input current_config.json --type common.Config --output current_config.pb
configtxlator proto_encode --input bft_config.json --type common.Config --output updated_config.pb
configtxlator compute_update --channel_id $CHANNEL_NAME --original current_config.pb --updated updated_config.pb --output config_update.pb

peer channel signconfigtx -f config_update.pb
peer channel update -f config_update.pb -c $CHANNEL_NAME -o orderer.example.com:7050 --tls --cafile $ORDERER_CA

## Step 6: Restart and Validate

```bash
docker restart $(docker ps -q --filter name=orderer)

# Check logs for all orderers
for orderer in orderer1 orderer2 orderer3 orderer4; do
  echo "Checking logs for $orderer"
  docker logs -f $orderer 2>&1 | grep -E "SmartBFT-v3 is now servicing chain|Message from leader"
done
```

## Step 7: Exit Maintenance Mode

```bash
jq '.channel_group.groups.Orderer.values.ConsensusType.value.state = "STATE_NORMAL"' bft_config.json > normal_config.json

configtxlator proto_encode --input bft_config.json --type common.Config --output current_config.pb
configtxlator proto_encode --input normal_config.json --type common.Config --output updated_config.pb
configtxlator compute_update --channel_id $CHANNEL_NAME --original current_config.pb --updated updated_config.pb --output config_update.pb

peer channel signconfigtx -f config_update.pb
peer channel update -f config_update.pb -c $CHANNEL_NAME -o orderer.example.com:7050 --tls --cafile $ORDERER_CA
```

## Step 8: Verify Migration

```bash
peer channel fetch config latest_config.pb -o orderer.example.com:7050 -c $CHANNEL_NAME --tls --cafile $ORDERER_CA

configtxlator proto_decode --input latest_config.pb --type common.Block | jq '.data.data[0].payload.data.config.channel_group.groups.Orderer.values.ConsensusType, .data.data[0].payload.data.config.channel_group.groups.Orderer.values.Orderers'
```

## Notes:

1. Ensure all ordering nodes are running Fabric v3.0.0 or higher before migration.
2. This process should be performed during a maintenance window to minimize disruption.
3. The migration is irreversible. Ensure you have a proper backup before proceeding.
4. The number of consenters in the BFT configuration should be 3f + 1, where f is the number of tolerated failures. In this example, we use 4 consenters, tolerating 1 failure.
5. Test this process in a non-production environment first.
