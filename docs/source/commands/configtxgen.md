# configtxgen

The `configtxgen` command allows users to create and inspect channel config
related artifacts.  The content of the generated artifacts is dictated by the
contents of `configtx.yaml`.

## Syntax

The `configtxgen` tool has no sub-commands, but supports flags which can be set
to accomplish a number of tasks.

## configtxgen
```
Usage of configtxgen:
  -asOrg string
    	Performs the config generation as a particular organization (by name), only including values in the write set that org (likely) has privilege to set
  -channelID string
    	The channel ID to use in the configtx
  -configPath string
    	The path containing the configuration to use (if set)
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
for profile `SampleSingleMSPSoloV1_1`.

```
configtxgen -outputBlock genesis_block.pb -profile SampleSingleMSPSoloV1_1 -channelID orderer-system-channel
```

### Output a channel creation tx

Write a channel creation transaction to `create_chan_tx.pb` for profile
`SampleSingleMSPChannelV1_1`.

```
configtxgen -outputCreateChannelTx create_chan_tx.pb -profile SampleSingleMSPChannelV1_1 -channelID application-channel-1
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
anchor peers for organization Org1 as defined in profile
SampleSingleMSPChannelV1_1 based on `configtx.yaml`.

```
configtxgen -outputAnchorPeersUpdate anchor_peer_tx.pb -profile SampleSingleMSPChannelV1_1 -asOrg Org1
```

## Configuration

The `configtxgen` tool's output is largely controlled by the content of
`configtx.yaml`.  This file is searched for at `FABRIC_CFG_PATH` and must be
present for `configtxgen` to operate.

This configuration file may be edited, or, individual properties may be
overridden by setting environment variables, such as
`CONFIGTX_ORDERER_ORDERERTYPE=kafka`.

For many `configtxgen` operations, a profile name must be supplied.  Profiles
are a way to express multiple similar configurations in a single file.  For
instance, one profile might define a channel with 3 orgs, and another might
define one with 4 orgs.  To accomplish this without the length of the file
becoming burdensome, `configtx.yaml` depends on the standard YAML feature of
anchors and references.  Base parts of the configuration are tagged with an
anchor like `&OrdererDefaults` and then merged into a profile with a reference
like `<<: *OrdererDefaults`.  Note, when `configtxgen` is operating under a
profile, environment variable overrides do not need to include the profile
prefix and may be referenced relative to the root element of the profile.  For
instance, do not specify
`CONFIGTX_PROFILE_SAMPLEINSECURESOLO_ORDERER_ORDERERTYPE`,
instead simply omit the profile specifics and use the `CONFIGTX` prefix
followed by the elements relative to the profile name such as
`CONFIGTX_ORDERER_ORDERERTYPE`.

Refer to the sample `configtx.yaml` shipped with Fabric for all possible
configuration options.  You may find this file in the `config` directory of
the release artifacts tar, or you may find it under the `sampleconfig` folder
if you are building from source.


<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
