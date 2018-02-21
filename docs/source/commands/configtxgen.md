configtxgen
=============

## Description

The `configtxgen` command allows users to create and inspect channel config
related artifacts.  The content of the generated artifacts is dictated by the
contents of `configtx.yaml`.

## Syntax

The `configtxgen` tool has no sub-commands, but supports flags which can be set
to accomplish a number of tasks.

```
Usage of configtxgen:
  -asOrg string
    	Performs the config generation as a particular organization (by name), only including values in the write set that org (likely) has privilege to set
  -channelID string
    	The channel ID to use in the configtx (default "testchainid")
  -inspectBlock string
    	Prints the configuration contained in the block at the specified path
  -inspectChannelCreateTx string
    	Prints the configuration contained in the transaction at the specified path
  -outputAnchorPeersUpdate string
    	Creates an config update to update an anchor peer (works only with the default channel creation, and only for the first update)
  -outputBlock string
    	The path to write the genesis block to (if set)
  -outputCreateChannelTx string
    	The path to write a channel creation configtx to (if set)
  -printOrg string
    	Prints the definition of an organization as JSON. (useful for adding an org to a channel manually)
  -profile string
    	The profile from configtx.yaml to use for generation. (default "SampleInsecureSolo")
  -version
    	Show version information
```

## Usage

### Output a genesis block

Write a genesis block to `genesis_block.pb` for channel `orderer-system-channel`
for profile `SampleSingleMSPSolo`.

```
configtxgen -outputBlock genesis_block.pb -profile SampleSingleMSPSolo -channelID orderer-system-channel
```

### Output a channel creation tx

Write a channel creation transaction to `create_chan_tx.pb` for profile
`SampleSingleMSPChannel`.

```
configtxgen -outputCreateChannelTx create_chan_tx.pb -profile SampleSingleMSPChannel -channelID application-channel-1
```

### Inspect a genesis block

Print the contents of a genesis block named `genesis_block.pb` to the screen as
JSON.

```
configtxgen -inspectBlock genesis_block.pb
```

### Inspect a channel creation tx

Print the contents of a channel creation tx named `create_chan_tx.pb` to the
screen as JSON.

```
configtxgen -inspectChannelCreateTx create_chan_tx.pb
```

### Print an organization definition

Construct an organization definition based on the parameters such as MSPDir
from `configtx.yaml` and print it as JSON to the screen. (This output is useful
for channel reconfiguration workflows, such as adding a member).

```
configtxgen -printOrg Org1
```

### Output anchor peer tx

Output a configuration update transaction to `anchor_peer_tx.pb` which sets the
anchor peers for organization Org1 as defined in profile SampleSingleMSPChannel
based on `configtx.yaml`.

```
configtxgen -outputAnchorPeersUpdate anchor_peer_tx.pb -profile SampleSingleMSPChannel -asOrg Org1
```
