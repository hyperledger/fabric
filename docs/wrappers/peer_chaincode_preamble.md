# peer chaincode

The `peer chaincode` command allows users to invoke and query chaincode.

## Syntax

The `peer chaincode` command has the following subcommands:

  * invoke
  * query

The subcommands take a constructor flag (`-c` or `--ctor`) to pass arguments to a chaincode.
The value must be a JSON string that has either key 'Args' or 'Function' and 'Args'.
These keys are case-insensitive.

If the constructor JSON string only has the Args key, the key value is an array, where the
first array element is the target function to call, and the subsequent elements
are arguments of the function. If the JSON string has both 'Function' and
'Args', the value of Function is the target function to call, and the value of
Args is an array of arguments of the function. For instance,
`{"Args":["GetAllAssets"]}` is equivalent to
`{"Function":"GetAllAssets", "Args":[]}`.

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

Flags of type stringArray are to be repeated rather than concatenating their
values. For example, you will use `--peerAddresses localhost:9051
--peerAddresses localhost:7051` rather than `--peerAddresses "localhost:9051
localhost:7051"`.
