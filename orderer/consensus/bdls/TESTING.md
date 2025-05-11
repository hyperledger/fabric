# Testing Dynamic Membership in BDLS Consensus

This document describes how to test the dynamic membership management feature in BDLS consensus.

## Prerequisites
- Hyperledger Fabric development environment
- Go 1.16 or later
- Docker and Docker Compose

## Unit Tests
Run the unit tests:
```bash
go test -v github.com/hyperledger/fabric/orderer/consensus/bdls -run TestConfigUpdate
```

## Integration Tests
1. Start a 4-node network:
```bash
cd test/network
./network.sh up -o bdls -n 4
```

2. Add a new node:
```bash
# Generate the config block
configtxgen -profile BDLSOrdererGenesis -outputBlock config_block.pb -channelID syschannel

# Join the new node
osnadmin channel join --channelID syschannel --config-block config_block.pb -o localhost:7053
```

3. Verify node addition:
- Check logs for successful certificate validation
- Verify connection establishment
- Confirm consensus participation

4. Remove a node:
```bash
# Generate the removal config
configtxgen -profile BDLSOrdererGenesis -outputBlock remove_node.pb -channelID syschannel

# Update the channel configuration
osnadmin channel update --channelID syschannel --config-block remove_node.pb -o localhost:7053
```

5. Verify node removal:
- Check logs for graceful shutdown
- Verify cluster reconfiguration
- Confirm continued consensus operation

## Troubleshooting
- If certificate validation fails, check the certificate format and expiration
- For connection issues, verify network connectivity and port accessibility
- Check logs at `/var/hyperledger/production/orderer/logs`
