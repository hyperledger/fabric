# peer snapshot

The `peer snapshot` command allows administrators to perform snapshot related
operations on a peer, such as submit a snapshot request, cancel a snapshot request
and list pending requests. Once a snapshot request is submitted for a specified
block number, the snapshot will be automatically generated when the block number is
committed on the channel.

## Syntax

The `peer snapshot` command has the following subcommands:

  * cancelrequest
  * listpending
  * submitrequest
