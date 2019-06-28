# peer node

The `peer node` command allows an administrator to start a peer node or check
the status of a peer node.

## Syntax

The `peer node` command has the following subcommands:

  * start
  * status

## peer node start
```
Starts a node that interacts with the network.

Usage:
  peer node start [flags]

Flags:
  -h, --help                help for start
      --peer-chaincodedev   Whether peer in chaincode development mode
```


## peer node status
```
Returns the status of the running node.

Usage:
  peer node status [flags]

Flags:
  -h, --help   help for status
```


## peer node reset
```
Resets all channels to the genesis block. When the command is executed, the peer must be offline. When the peer starts after the reset, it will receive blocks starting with block number one from an orderer or another peer to rebuild the block store and state database.

Usage:
  peer node reset [flags]

Flags:
  -h, --help   help for reset
```


## peer node rollback
```
Rolls back a channel to a specified block number. When the command is executed, the peer must be offline. When the peer starts after the rollback, it will receive blocks, which got removed during the rollback, from an orderer or another peer to rebuild the block store and state database.

Usage:
  peer node rollback [flags]

Flags:
  -b, --blockNumber uint   Block number to which the channel needs to be rolled back to.
  -c, --channelID string   Channel to rollback.
  -h, --help               help for rollback
```

## Example Usage

### peer node start example

The following command:

```
peer node start --peer-chaincodedev
```

starts a peer node in chaincode development mode. Normally chaincode containers are started
and maintained by peer. However in chaincode development mode, chaincode is built and started by the user. This mode is useful during chaincode development phase for iterative development.
See more information on development mode in the [chaincode tutorial](../chaincode4ade.html).

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
