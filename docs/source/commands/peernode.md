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
  -o, --orderer string      Ordering service endpoint (default "orderer:7050")
      --peer-chaincodedev   Whether peer in chaincode development mode

Global Flags:
      --logging-level string   Default logging level and overrides, see core.yaml for full syntax
```


## peer node status
```
Returns the status of the running node.

Usage:
  peer node status [flags]

Flags:
  -h, --help   help for status

Global Flags:
      --logging-level string   Default logging level and overrides, see core.yaml for full syntax
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
