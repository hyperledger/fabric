# osnadmin channel

The `osnadmin channel` command allows administrators to perform channel-related
operations on an orderer, such as joining a channel, listing the channels an
orderer has joined, and removing a channel. The channel participation API must
be enabled and the Admin endpoint must be configured in the `orderer.yaml` for
each orderer.

## Syntax

The `osnadmin channel` command has the following subcommands:

  * join
  * list
  * remove
