# peer node

## Description

The `peer node` subcommand allows an administrator to start a peer node or check
the status of a peer node.

## Syntax

The `peer node` subcommand has the following syntax:

```
peer node start [flags]
peer node status
```

## peer node start

### Start Description
The `peer node start` command allows administrators to start the peer node process.

The peer node process can be configured using configuration file *core.yaml*, which
must be located in the directory specified by the environment variable **FABRIC_CFG_PATH**.
For docker deployments, *core.yaml* is pre-configured in the peer container **FABRIC_CFG_PATH** directory.
For native binary deployments, *core.yaml* is included with the release artifact distribution.
The configuration properties located in *core.yaml* can be overridden using environment variables.
For example, `peer.mspConfigPath` configuration property can be specified by defining
**CORE_PEER_MSPCONFIGPATH** environment variable, where **CORE_** is the prefix for the
environment variables.

### Start Syntax
The `peer node start` command has the following syntax:

```
peer node start [flags]

```

### Start Flags
The `peer node start` command has the following command specific flag:

* `--peer-chaincodedev`

  starts peer node in chaincode development mode. Normally chaincode containers are started
  and maintained by peer. However in devlopment mode, chaincode is built and started by the user.
  This mode is useful during chaincode development phase for iterative development.
  See more information on development mode in the [chaincode tutorial](../chaincode4ade.html).

The global `peer` command flags also apply as described in the `peer command` [topic](./peercommand.html):

* --logging-level

## peer node status

### Status Description
The `peer node status` command allows administrators to see the status of the peer node process.
It will show the status of the peer node process running at the `peer.address` specified in the
peer configuration, or overridden by **CORE_PEER_ADDRESS** environment variable.

### Status Syntax
The `peer node status` command has the following syntax:

```
peer node status
```

### Status Flags
The `peer node status` command has no command specific flags.
