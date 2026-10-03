# cli

## Description

 The `cli` command has five different subcommands, each of which allows
 administrators to perform a specific set of tasks related to a cli.  For
 example, you can use the `cli channel` subcommand to join a cli to a channel,
 or the `cli chaincode` command to deploy a smart contract chaincode to a
 cli.

## Syntax

The `cli` command has five different subcommands within it:

```
cli chaincode [option] [flags]
cli channel   [option] [flags]
cli version   [option] [flags]
```

Each subcommand has different options available, and these are described in
their own dedicated topic. For brevity, we often refer to a command (`cli`), a
subcommand (`channel`), or subcommand option (`fetch`) simply as a **command**.

If a subcommand is specified without an option, then it will return some high
level help text as described in the `--help` flag below.

## Flags

Each `cli` subcommand has a specific set of flags associated with it, many of
which are designated *global* because they can be used in all subcommand
options. These flags are described with the relevant `cli` subcommand.

The top level `cli` command has the following flag:

* `--help`

  Use `--help` to get brief help text for any `cli` command. The `--help` flag
  is very useful -- it can be used to get command help, subcommand help, and
  even option help.

  For example
  ```
  cli --help
  cli channel --help
  cli channel list --help

  ```
  See individual `cli` subcommands for more detail.

## Usage

Here is an example using the available flag on the `cli` command.

* Using the `--help` flag on the `cli channel join` command.

  ```
  cli channel join --help

  Joins the cli to a channel.

  Usage:
    cli channel join [flags]

  Flags:
    -b, --blockpath string   Path to file containing genesis block
    -h, --help               help for join

  Global Flags:
        --cafile string                       Path to file containing PEM-encoded trusted certificate(s) for the ordering endpoint
        --certfile string                     Path to file containing PEM-encoded X509 public key to use for mutual TLS communication with the orderer endpoint
        --clientauth                          Use mutual TLS when communicating with the orderer endpoint
        --connTimeout duration                Timeout for client to connect (default 3s)
        --keyfile string                      Path to file containing PEM-encoded private key to use for mutual TLS communication with the orderer endpoint
    -o, --orderer string                      Ordering service endpoint
        --ordererTLSHostnameOverride string   The hostname override to use when validating the TLS connection to the orderer.
        --tls                                 Use TLS when communicating with the orderer endpoint

  ```
  This shows brief help syntax for the `cli channel join` command.
