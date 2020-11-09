# osnadmin channel

The `osnadmin channel` command allows administrators to perform channel-related
operations on an orderer, such as joining a channel, listing the channels an
orderer has joined, and removing a channel. The channel participation API must
be enabled and the Admin endpoint must be configured in the `orderer.yaml` for
each orderer.

*Note: For a network using a system channel, `list` (for all channels) and
`remove` (for the system channel) are the only supported operations. Any other
attempted operation will return an error.

## Syntax

The `osnadmin channel` command has the following subcommands:

  * join
  * list
  * remove
