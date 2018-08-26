# peer

## Description

 The `peer` command has five different subcommands, each of which allows
 administrators to perform a specific set of tasks related to a peer.  For
 example, you can use the `peer channel` subcommand to join a peer to a channel,
 or the `peer  chaincode` command to deploy a smart contract chaincode to a
 peer.

## Syntax

The `peer` command has five different subcommands within it:

```
peer chaincode [option] [flags]
peer channel   [option] [flags]
peer logging   [option] [flags]
peer node      [option] [flags]
peer version   [option] [flags]
```

Each subcommand has different options available, and these are described in
their own dedicated topic. For brevity, we often refer to a command (`peer`), a
subcommand (`channel`), or subcommand option (`fetch`) simply as a **command**.

If a subcommand is specified without an option, then it will return some high
level help text as described in the `--help` flag below.

## Flags

Each `peer` subcommand has a specific set of flags associated with it, many of
which are designated *global* because they can be used in all subcommand
options. These flags are described with the relevant `peer` subcommand.

The top level `peer` command has the following flags:

* `--help`

  Use `--help` to get brief help text for any `peer` command. The `--help` flag
  is very useful -- it can be used to get command help, subcommand help, and
  even option help.

  For example
  ```
  peer --help
  peer channel --help
  peer channel list --help

  ```
  See individual `peer` subcommands for more detail.

* `--logging-level <string>`

  This flag sets the logging level for a peer when it is started.

  There are six possible values for  `<string>` : `debug`, `info`, `warning`,
  `error`, `panic`, and `fatal`.

  If `logging-level` is not explicitly specified, then it is taken from the
  `CORE_LOGGING_LEVEL` environment variable if it is set. If
  `CORE_LOGGING_LEVEL` is not set then the file `sampleconfig/core.yaml` is used
  to determined the logging level for the peer.

  You can find the current logging level for a specific component on the peer by
  running `peer logging getlevel <component-name>`.

* `--version`

  Use this flag to show detailed information about how the peer was built. This
  flag cannot be applied to `peer` subcommands or their options.

## Usage

Here's some examples using the different available flags on the `peer` command.

* Using the `--help` flag on the `peer channel join` command.

  ```
  peer channel join --help

  Joins the peer to a channel.

  Usage:
    peer channel join [flags]

  Flags:
    -b, --blockpath string   Path to file containing genesis block

  Global Flags:
        --cafile string                       Path to file containing PEM-encoded trusted certificate(s) for the ordering endpoint
        --certfile string                     Path to file containing PEM-encoded X509 public key to use for mutual TLS communication with the orderer endpoint
        --clientauth                          Use mutual TLS when communicating with the orderer endpoint
        --keyfile string                      Path to file containing PEM-encoded private key to use for mutual TLS communication with the orderer endpoint
        --logging-level string                Default logging level and overrides, see core.yaml for full syntax
    -o, --orderer string                      Ordering service endpoint
        --ordererTLSHostnameOverride string   The hostname override to use when validating the TLS connection to the orderer.
        --tls                                 Use TLS when communicating with the orderer endpoint
    -v, --version                             Display the build version for this fabric peer

  ```
  This shows brief help syntax for the `peer channel join` command.

* Using the `--version` flag on the `peer` command.

  ```
  peer --version

  peer:
   Version: 1.1.0-alpha
   Go version: go1.9.2
   OS/Arch: linux/amd64
   Experimental features: false
   Chaincode:
    Base Image Version: 0.4.5
    Base Docker Namespace: hyperledger
    Base Docker Label: org.hyperledger.fabric
    Docker Namespace: hyperledger

  ```

  This shows that this peer was built using an alpha of Hyperledger Fabric
  version 1.1.0, compiled with GOLANG 1.9.2. It can be used on Linux operating
  systems with AMD64 compatible instruction sets.
