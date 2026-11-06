# peer

## Description

 The `peer` command has five different subcommands, each of which allows
 administrators to perform a specific set of tasks related to a peer.

## Syntax

The `peer` command has five different subcommands within it:

```
peer node      [option] [flags]
peer version   [option] [flags]
```

Each subcommand has different options available, and these are described in
their own dedicated topic. For brevity, we often refer to a command (`peer`), a
subcommand (`node`), or subcommand option (`unjoin`) simply as a **command**.

If a subcommand is specified without an option, then it will return some high
level help text as described in the `--help` flag below.

## Flags

Each `peer` subcommand has a specific set of flags associated with it, many of
which are designated *global* because they can be used in all subcommand
options. These flags are described with the relevant `peer` subcommand.

The top level `peer` command has the following flag:

* `--help`

  Use `--help` to get brief help text for any `peer` command. The `--help` flag
  is very useful -- it can be used to get command help, subcommand help, and
  even option help.

  For example
  ```
  peer --help
  peer node --help
  peer node unjoin --help

  ```
  See individual `peer` subcommands for more detail.

## Usage

Here is an example using the available flag on the `peer` command.

* Using the `--help` flag on the `peer node unjoin` command.

  ```
  peer node unjoin --help

  Unjoin the peer from a channel.  When the command is executed, the peer must be offline.

  Usage:
  peer node unjoin [flags]

  Flags:
  -c, --channelID string   Channel to unjoin.
  -h, --help               help for unjoin

  ```
  This shows brief help syntax for the `peer node unjoin` command.
