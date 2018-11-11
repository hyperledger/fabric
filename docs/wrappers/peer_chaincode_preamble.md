# peer chaincode

The `peer chaincode` command allows administrators to perform chaincode
related operations on a peer, such as installing, instantiating, invoking,
packaging, querying, and upgrading chaincode.

## Syntax

The `peer chaincode` command has the following subcommands:

  * install
  * instantiate
  * invoke
  * list
  * package
  * query
  * signpackage
  * upgrade

The different subcommand options (install, instantiate...) relate to the
different chaincode operations that are relevant to a peer. For example, use the
`peer chaincode install` subcommand option to install a chaincode on a peer, or
the `peer chaincode query` subcommand option to query a chaincode for the
current value on a peer's ledger.

Each peer chaincode subcommand is described together with its options in its own
section in this topic.

## Flags

Each `peer chaincode` subcommand has both a set of flags specific to an
individual subcommand, as well as a set of global flags that relate to all
`peer chaincode` subcommands. Not all subcommands would use these flags.
For instance, the `query` subcommand does not need the `--orderer` flag.

The individual flags are described with the relevant subcommand. The global
flags are

* `--cafile <string>`

  Path to file containing PEM-encoded trusted certificate(s) for the ordering
  endpoint

* `--certfile <string>`

  Path to file containing PEM-encoded X509 public key to use for mutual TLS
  communication with the orderer endpoint

* `--keyfile <string>`

  Path to file containing PEM-encoded private key to use for mutual TLS
  communication with the orderer endpoint

* `-o` or `--orderer <string>`

  Ordering service endpoint specified as `<hostname or IP address>:<port>`

* `--ordererTLSHostnameOverride <string>`

  The hostname override to use when validating the TLS connection to the orderer

* `--tls`

  Use TLS when communicating with the orderer endpoint

* `--transient <string>`

  Transient map of arguments in JSON encoding
