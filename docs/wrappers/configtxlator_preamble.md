# configtxlator

The `configtxlator` command allows users to translate between protobuf and JSON
versions of fabric data structures and create config updates.  The command may
either start a REST server to expose its functions over HTTP or may be utilized
directly as a command line tool.

## Syntax

The `configtxlator` tool has five sub-commands, as follows:

  * start
  * proto_encode
  * proto_decode
  * compute_update
  * version
