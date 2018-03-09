# peer chaincode

## Description

The `peer chaincode` subcommand allows administrators to perform chaincode
related operations on a peer, such as installing, instantiating, invoking,
packaging, querying, and upgrading chaincode.

## Syntax

The `peer chaincode` subcommand has the following syntax:

```
peer chaincode install      [flags]
peer chaincode instantiate  [flags]
peer chaincode invoke       [flags]
peer chaincode list         [flags]
peer chaincode package      [flags]
peer chaincode query        [flags]
peer chaincode signpackage  [flags]
peer chaincode upgrade      [flags]
```

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

  Ordering service endpoint specifed as `<hostname or IP address>:<port>`

* `--ordererTLSHostnameOverride <string>`

  The hostname override to use when validating the TLS connection to the orderer

* `--tls`

  Use TLS when communicating with the orderer endpoint

* `--transient <string>`

  Transient map of arguments in JSON encoding

* `--logging-level <string>`

  Default logging level and overrides, see core.yaml for full syntax

## peer chaincode install

### Install Description

The `peer chaincode install` command allows administrators to install chaincode
onto the filesystem of a peer.

### Install Syntax

The `peer chaincode install` command has the following syntax:

```
peer chaincode install [flags]
```

Note: An install can also be performed using a chaincode packaged via the
`peer chaincode package` command (see the `peer chaincode package` section below
for further details on packaging a chaincode for installation). The syntax using
a chaincode package is as follows:

```
peer chaincode install [chaincode-package-file]
```

where `[chaincode-package-file]` is the output file from the
`peer chaincode package` command.

### Install Flags

The `peer chaincode install` command has the following command-specific flags:

  * `-c, --ctor <string>`

    Constructor message for the chaincode in JSON format (default "{}")

  * `-l, --lang <string>`

    Language the chaincode is written in (default "golang")

  * `-n, --name <string>`

    Name of the chaincode that is being installed. It may consist of
    alphanumerics, dashes, and underscores

  * `-p, --path <string>`

    Path to the chaincode that is being installed. For Golang (-l golang)
    chaincodes, this is the path relative to the GOPATH. For Node.js (-l node)
    chaincodes, this is either the absolute path or the relative path from where
    the install command is being performed

  * `-v, --version <string>`

    Version of the chaincode that is being installed. It may consist of
    alphanumerics, dashes, underscores, periods, and plus signs

### Install Usage

Here are some examples of the `peer chaincode install` command:

  * To install chaincode named `mycc` at version `1.0`:

    ```
    peer chaincode install -n mycc -v 1.0 -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02

    .
    .
    .
    2018-02-22 16:33:52.998 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 003 Using default escc
    2018-02-22 16:33:52.998 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 004 Using default vscc
    .
    .
    .
    2018-02-22 16:33:53.194 UTC [chaincodeCmd] install -> DEBU 010 Installed remotely response:<status:200 payload:"OK" >
    2018-02-22 16:33:53.194 UTC [main] main -> INFO 011 Exiting.....

    ```

    Here you can see that the install completed successfully based on the log
    message:

    ```
    2018-02-22 16:33:53.194 UTC [chaincodeCmd] install -> DEBU 010 Installed remotely response:<status:200 payload:"OK" >

    ```

  * To install chaincode package `ccpack.out` generated with the `package`
    subcommand

    ```
    peer chaincode install ccpack.out

    .
    .
    .
    2018-02-22 18:18:05.584 UTC [chaincodeCmd] install -> DEBU 005 Installed remotely response:<status:200 payload:"OK" >
    2018-02-22 18:18:05.584 UTC [main] main -> INFO 006 Exiting.....
    ```

    Here you can see that the install completed successfully based on the log
    message:

    ```
    2018-02-22 18:18:05.584 UTC [chaincodeCmd] install -> DEBU 005 Installed remotely response:<status:200 payload:"OK" >

    ```

## peer chaincode instantiate

### Instantiate Description

The `peer chaincode instantiate` command allows administrators to instantiate
chaincode on a channel of which the peer is a member.

### Instantiate Syntax

The `peer chaincode instantiate` command has the following syntax:

```
peer chaincode instantiate [flags]

```

### Instantiate Flags

The `peer chaincode instantiate` command has the following command-specific
flags:

  * `-C, --channelID <string>`

    Name of the channel where the chaincode should be instantiated

  * `-c, --ctor <string>`

    Constructor message for the chaincode in JSON format (default "{}")

  * `-E, --escc <string>`

    Name of the endorsement system chaincode to be used for this chaincode (default "escc")

  * `-n, --name <string>`

    Name of the chaincode that is being instantiated

  * `-P, --policy <string>`

    Endorsement policy associated to this chaincode. By default fabric
    will generate an endorsement policy equivalent to "any member from
    the organizations currently in the channel"

  * `-v, --version <string>`

    Version of the chaincode that is being instantiated

  * `-V, --vscc <string>`

    Name of the verification system chaincode to be used for this chaincode (default "vscc")


The global `peer` command flags also apply:

  * `--cafile <string>`
  * `--certfile <string>`
  * `--keyfile <string>`
  * `-o, --orderer <string>`
  * `--ordererTLSHostnameOverride <string>`
  * `--tls`
  * `--transient <string>`

```
If `--orderer` flag is not specified, the command will attempt to retrieve
the orderer information for the channel from the peer before issuing the
instantiate command.
```

### Instantiate Usage

Here are some examples of the `peer chaincode instantiate` command, which
instantiates the chaincode named `mycc` at version `1.0` on channel
`mychannel`:

  * Using the `--tls` and `--cafile` global flags to instantiate the chaincode
    in a network with TLS enabled:

    ```
    export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
    peer chaincode instantiate -o orderer.example.com:7050 --tls --cafile $ORDERER_CA -C mychannel -n mycc -v 1.0 -c '{"Args":["init","a","100","b","200"]}' -P "OR	('Org1MSP.peer','Org2MSP.peer')"

    2018-02-22 16:33:53.324 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 001 Using default escc
    2018-02-22 16:33:53.324 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 002 Using default vscc
    2018-02-22 16:34:08.698 UTC [main] main -> INFO 003 Exiting.....

    ```

  * Using only the command-specific options to instantiate the chaincode in a
    network with TLS disabled:

    ```
    peer chaincode instantiate -o orderer.example.com:7050 -C mychannel -n mycc -v 1.0 -c '{"Args":["init","a","100","b","200"]}' -P "OR	('Org1MSP.peer','Org2MSP.peer')"


    2018-02-22 16:34:09.324 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 001 Using default escc
    2018-02-22 16:34:09.324 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 002 Using default vscc
    2018-02-22 16:34:24.698 UTC [main] main -> INFO 003 Exiting.....
    ```


## peer chaincode invoke

### Invoke Description

The `peer chaincode invoke` command allows administrators to call chaincode
functions on a peer using the supplied arguments. The CLI invokes chaincode by
sending a transaction proposal to a peer. The peer will execute the chaincode
and send the endorsed proposal response (or error) to the CLI. On receipt of
a endorsed proposal response, the CLI will construct a transaction with it
and send it to the orderer.

### Invoke Syntax

The `peer chaincode invoke` command has the following syntax:

```
peer chaincode invoke [flags]
```

### Invoke Flags

The `peer chaincode invoke` command has the following command-specific flags:

  * `-C, --channelID <string>`

    Name of the chaincode that is being invoked

  * `-c, --ctor <string>`

    Constructor message for the chaincode in JSON format (default "{}")

  * `-n, --name <string>`

    Name of the chaincode that is being invoked

The global `peer` command flags also apply:

  * `--cafile <string>`
  * `--certfile <string>`
  * `--keyfile <string>`
  * `-o, --orderer <string>`
  * `--ordererTLSHostnameOverride <string>`
  * `--tls`
  * `--transient <string>`

```
If `--orderer` flag is not specified, the command will attempt to retrieve
the orderer information for the channel from the peer before issuing the
invoke command.
```

### Invoke Usage

Here is an example of the `peer chaincode invoke` command, which
invokes the chaincode named `mycc` at version `1.0` on channel
`mychannel`, requesting to move 10 units from variable `a` to variable `b`:

* ```
  peer chaincode invoke -o orderer.example.com:7050 -C mychannel -n mycc -c '{"Args":["invoke","a","b","10"]}'

  2018-02-22 16:34:27.069 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 001 Using default escc
  2018-02-22 16:34:27.069 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 002 Using default vscc
  .
  .
  .
  2018-02-22 16:34:27.106 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> DEBU 00a ESCC invoke result: version:1 response:<status:200 message:"OK" > payload:"\n \237mM\376? [\214\002 \332\204\035\275q\227\2132A\n\204&\2106\037W|\346#\3413\274\022Y\nE\022\024\n\004lscc\022\014\n\n\n\004mycc\022\002\010\003\022-\n\004mycc\022%\n\007\n\001a\022\002\010\003\n\007\n\001b\022\002\010\003\032\007\n\001a\032\00290\032\010\n\001b\032\003210\032\003\010\310\001\"\013\022\004mycc\032\0031.0" endorsement:<endorser:"\n\007Org1MSP\022\262\006-----BEGIN CERTIFICATE-----\nMIICLjCCAdWgAwIBAgIRAJYomxY2cqHA/fbRnH5a/bwwCgYIKoZIzj0EAwIwczEL\nMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG\ncmFuY2lzY28xGTAXBgNVBAoTEG9yZzEuZXhhbXBsZS5jb20xHDAaBgNVBAMTE2Nh\nLm9yZzEuZXhhbXBsZS5jb20wHhcNMTgwMjIyMTYyODE0WhcNMjgwMjIwMTYyODE0\nWjBwMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZvcm5pYTEWMBQGA1UEBxMN\nU2FuIEZyYW5jaXNjbzETMBEGA1UECxMKRmFicmljUGVlcjEfMB0GA1UEAxMWcGVl\ncjAub3JnMS5leGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABDEa\nWNNniN3qOCQL89BGWfY39f5V3o1pi//7JFDHATJXtLgJhkK5KosDdHuKLYbCqvge\n46u3AC16MZyJRvKBiw6jTTBLMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMBAf8EAjAA\nMCsGA1UdIwQkMCKAIN7dJR9dimkFtkus0R5pAOlRz5SA3FB5t8Eaxl9A7lkgMAoG\nCCqGSM49BAMCA0cAMEQCIC2DAsO9QZzQmKi8OOKwcCh9Gd01YmWIN3oVmaCRr8C7\nAiAlQffq2JFlbh6OWURGOko6RckizG8oVOldZG/Xj3C8lA==\n-----END CERTIFICATE-----\n" signature:"0D\002 \022_\342\350\344\231G&\237\n\244\375\302J\220l\302\345\210\335D\250y\253P\0214:\221e\332@\002 \000\254\361\224\247\210\214L\277\370\222\213\217\301\r\341v\227\265\277\336\256^\217\336\005y*\321\023\025\367" >
  2018-02-22 16:34:27.107 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 00b Chaincode invoke successful. result: status:200
  2018-02-22 16:34:27.107 UTC [main] main -> INFO 00c Exiting.....

  ```

  Here you can see that the invoke was submitted successfully based on the log
  message:

  ```
  2018-02-22 16:34:27.107 UTC [chaincodeCmd] chaincodeInvokeOrQuery -> INFO 00b Chaincode invoke successful. result: status:200

  ```

```
A successful response indicates that the transaction was submitted for ordering
successfully. The transaction will then be added to a block and, finally, validated
or invalidated by each peer on the channel.
```

## peer chaincode list

### List Description

The `peer chaincode list` command allows administrators to list the chaincodes
installed on a peer or the chaincodes instantiated on a channel of which the
peer is a member.

### List Syntax

The `peer chaincode list` command has the following syntax:

```
peer chaincode list [--installed|--instantiated -C <channel-name>]
```

### List Flags

The `peer chaincode instantiate` command has the following command-specific
flags:

* `-C, --channelID <string>`

  Name of the channel to list instantiated chaincodes for

* `--installed`

  Use this flag to list the installed chaincodes on a peer

* `--instantiated`

  Use this flag to list the instantiated chaincodes on a channel that the peer
  is a member of

### List Usage

Here are some examples of the `peer chaincode list ` command:

* Using the `--installed` flag to list the chaincodes installed on a peer.

  ```
  peer chaincode list --installed

  Get installed chaincodes on peer:
  Name: mycc, Version: 1.0, Path: github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02, Id: 8cc2730fdafd0b28ef734eac12b29df5fc98ad98bdb1b7e0ef96265c3d893d61
  2018-02-22 17:07:13.476 UTC [main] main -> INFO 001 Exiting.....

  ```

  You can see that the peer has installed a chaincode called `mycc` which is at
  version `1.0`.

* Using the `--instantiated` in combination with the `-C` (channel ID) flag to
  list the chaincodes instantiated on a channel.

  ```
  peer chaincode list --instantiated -C mychannel

  Get instantiated chaincodes on channel mychannel:
  Name: mycc, Version: 1.0, Path: github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02, Escc: escc, Vscc: vscc
  2018-02-22 17:07:42.969 UTC [main] main -> INFO 001 Exiting.....

  ```

  You can see that chaincode `mycc` at version `1.0` is instantiated on
  channel `mychannel`.


## peer chaincode package

### Package Description

The `peer chaincode package` command allows administrators to package the
materials necessary to perform a chaincode install. This ensures the same
chaincode package can be consistently installed on multiple peers.

### Package Syntax

The `peer chaincode package` command has the following syntax:

```
peer chaincode package [output-file] [flags]
```

### Package Flags

The `peer chaincode package` command has the following command-specific flags:

  * `-c, --ctor <string>`

    Constructor message for the chaincode in JSON format (default "{}")

  * `-i, --instantiate-policy <string>`

    Instantiation policy for the chaincode. Currently only policies that
    require utmost 1 signature (e.g., "OR ('Org1MSP.peer','Org2MSP.peer')") are
    supported.

  * `-l, --lang <string>`

    Language the chaincode is written in (default "golang")

  * `-n, --name <string>`

    Name of the chaincode that is being installed. It may consist of
    alphanumerics, dashes, and underscores

  * `-p, --path <string>`

    Path to the chaincode that is being packaged. For Golang (-l golang)
    chaincodes, this is the path relative to the GOPATH. For Node.js (-l node)
    chaincodes, this is either the absolute path or the relative path from where
    the package command is being performed

  * `-s, --cc-package`

    Create a package for storing chaincode ownership information in addition
    the raw chaincode deployment spec (however, see note below.)

  * `-S, --sign`

    Used with the `-s` flag, specify this flag to add owner endorsements to the
    package using the local MSP (however, see note below.)

  * `-v, --version <string>`

    Version of the chaincode that is being installed. It may consist of
    alphanumerics, dashes, underscores, periods, and plus signs

```
The metadata from `-s` and `-S` commands are not currently used. These commands
are meant for future extensions and will likely undergo implementation changes.
It is recommended that they are not used.
```

### Package Usage

Here is an example of the `peer chaincode package` command, which
packages the chaincode named `mycc` at version `1.1`, creates the chaincode
deployment spec, signs the package using the local MSP, and outputs it as
`ccpack.out`:

* ```
  peer chaincode package ccpack.out -n mycc -p github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02 -v 1.1 -s -S

  .
  .
  .
  2018-02-22 17:27:01.404 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 003 Using default escc
  2018-02-22 17:27:01.405 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 004 Using default vscc
  .
  .
  .
  2018-02-22 17:27:01.879 UTC [chaincodeCmd] chaincodePackage -> DEBU 011 Packaged chaincode into deployment spec of size <3426>, with args = [ccpack.out]
  2018-02-22 17:27:01.879 UTC [main] main -> INFO 012 Exiting.....

  ```

## peer chaincode query

### Query Description

The `peer chaincode query` command allows the chaincode to be queried by
calling the `Invoke` method on the chaincode. The difference between the
`query` and the `invoke` subcommands is that, on successful response, `invoke`
proceeds to submit a transaction to the orderer whereas `query` just outputs
the response, successful or otherwise,  to stdout.

### Query Syntax

The `peer chaincode query` command has the following syntax:

```
peer chaincode query [flags]
```

### Query Flags

The `peer chaincode query` command has the following command-specific flags:

  * `-C, --channelID <string>`

    Name of the channel where the chaincode should be queried

  * `-c, --ctor <string>`

    Constructor message for the chaincode in JSON format (default "{}")

  * `-n, --name <string>`

    Name of the chaincode that is being queried

  * `-r --raw`

    Output the query value as raw bytes (default)

  * `-x --hex`

    Output the query value byte array in hexadecimal. Incompatible with --raw

The global `peer` command flag also applies:

  * `--transient <string>`

### Query Usage

Here is an example of the `peer chaincode query` command, which queries the
peer ledger for the chaincode named `mycc` at version `1.0` for the value of
variable `a`:

* ```
  peer chaincode query -C mychannel -n mycc -c '{"Args":["query","a"]}'

  2018-02-22 16:34:30.816 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 001 Using default escc
  2018-02-22 16:34:30.816 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 002 Using default vscc
  Query Result: 90

  ```

  You can see from the output that variable `a` had a value of 90 at the time of
  the query.

## peer chaincode signpackage

### signpackage Description

The `peer chaincode signpackage` command is used to add a signature to a given
chaincode package created with the `peer chaincode package` command using `-s`
and `-S` options.

### signpackge Syntax

The `peer chaincode signpackage` command has the following syntax:

```
peer chaincode signpackage <inputpackage> <outputpackage>
```

### signpackage Usage

Here is an example of the `peer chaincode signpackage` command, which accepts an
existing signed  package and creates a new one with signature of the local MSP
appended to it.

  ```
  peer chaincode signpackage ccwith1sig.pak ccwith2sig.pak
  Wrote signed package to ccwith2sig.pak successfully
  2018-02-24 19:32:47.189 EST [main] main -> INFO 002 Exiting.....
  ```

## peer chaincode upgrade

### Upgrade Description

The `peer chaincode upgrade` command allows administrators to upgrade the
chaincode instantiated on a channel to a newer version.

### Upgrade Syntax

The `peer chaincode upgrade` command has the following syntax:

```
peer chaincode upgrade [flags]
```

### Upgrade Flags

The `peer chaincode upgrade` command has the following command-specific flags:

  * `-C, --channelID <string>`

    Name of the channel where the chaincode should be upgraded

  * `-c, --ctor <string>`

    Constructor message for the chaincode in JSON format (default "{}")

  * `-E, --escc <string>`

    Name of the endorsement system chaincode to be used for this chaincode (default "escc")

  * `-n, --name <string>`

    Name of the chaincode that is being upgraded

  * `-P, --policy <string>`

    Endorsement policy associated to this chaincode. By default fabric
    will generate an endorsement policy equivalent to "any member from
    the organizations currently in the channel"

  * `-v, --version <string>`

    Version of the upgraded chaincode

  * `-V, --vscc <string>`

    Name of the verification system chaincode to be used for this chaincode (default "vscc")


The global `peer` command flags also apply:

* `--cafile <string>`
* `-o, --orderer <string>`
* `--tls`

```
If `--orderer` flag is not specified, the command will attempt to retrieve
the orderer information for the channel from the peer before issuing the
upgrade command.
```

### Upgrade Usage

Here is an example of the `peer chaincode upgrade` command, which
upgrades the chaincode named `mycc` at version `1.0` on channel
`mychannel` to version `1.1`, which contains a new variable `c`:

  * Using the `--tls` and `--cafile` global flags to upgrade the chaincode
    in a network with TLS enabled:

    ```
    export ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
    peer chaincode upgrade -o orderer.example.com:7050 --tls --cafile $ORDERER_CA -C mychannel -n mycc -v 1.2 -c '{"Args":["init","a","100","b","200","c","300"]}' -P "OR	('Org1MSP.peer','Org2MSP.peer')"

    .
    .
    .
    2018-02-22 18:26:31.433 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 003 Using default escc
    2018-02-22 18:26:31.434 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 004 Using default vscc
    2018-02-22 18:26:31.435 UTC [chaincodeCmd] getChaincodeSpec -> DEBU 005 java chaincode enabled
    2018-02-22 18:26:31.435 UTC [chaincodeCmd] upgrade -> DEBU 006 Get upgrade proposal for chaincode <name:"mycc" version:"1.1" >
    .
    .
    .
    2018-02-22 18:26:46.687 UTC [chaincodeCmd] upgrade -> DEBU 009 endorse upgrade proposal, get response <status:200 message:"OK" payload:"\n\004mycc\022\0031.1\032\004escc\"\004vscc*,\022\014\022\n\010\001\022\002\010\000\022\002\010\001\032\r\022\013\n\007Org1MSP\020\003\032\r\022\013\n\007Org2MSP\020\0032f\n \261g(^v\021\220\240\332\251\014\204V\210P\310o\231\271\036\301\022\032\205fC[|=\215\372\223\022 \311b\025?\323N\343\325\032\005\365\236\001XKj\004E\351\007\247\265fu\305j\367\331\275\253\307R\032 \014H#\014\272!#\345\306s\323\371\350\364\006.\000\356\230\353\270\263\215\217\303\256\220i^\277\305\214: \375\200zY\275\203}\375\244\205\035\340\226]l!uE\334\273\214\214\020\303\3474\360\014\234-\006\315B\031\022\010\022\006\010\001\022\002\010\000\032\r\022\013\n\007Org1MSP\020\001" >
    .
    .
    .
    2018-02-22 18:26:46.693 UTC [chaincodeCmd] upgrade -> DEBU 00c Get Signed envelope
    2018-02-22 18:26:46.693 UTC [chaincodeCmd] chaincodeUpgrade -> DEBU 00d Send signed envelope to orderer
    2018-02-22 18:26:46.908 UTC [main] main -> INFO 00e Exiting.....
    ```

  * Using only the command-specific options to upgrade the chaincode in a
    network with TLS disabled:

    ```peer chaincode upgrade -o orderer.example.com:7050 -C mychannel -n mycc -v 1.2 -c '{"Args":["init","a","100","b","200","c","300"]}' -P "OR	('Org1MSP.peer','Org2MSP.peer')"

    .
    .
    .
    2018-02-22 18:28:31.433 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 003 Using default escc
    2018-02-22 18:28:31.434 UTC [chaincodeCmd] checkChaincodeCmdParams -> INFO 004 Using default vscc
    2018-02-22 18:28:31.435 UTC [chaincodeCmd] getChaincodeSpec -> DEBU 005 java chaincode enabled
    2018-02-22 18:28:31.435 UTC [chaincodeCmd] upgrade -> DEBU 006 Get upgrade proposal for chaincode <name:"mycc" version:"1.1" >
    .
    .
    .
    2018-02-22 18:28:46.687 UTC [chaincodeCmd] upgrade -> DEBU 009 endorse upgrade proposal, get response <status:200 message:"OK" payload:"\n\004mycc\022\0031.1\032\004escc\"\004vscc*,\022\014\022\n\010\001\022\002\010\000\022\002\010\001\032\r\022\013\n\007Org1MSP\020\003\032\r\022\013\n\007Org2MSP\020\0032f\n \261g(^v\021\220\240\332\251\014\204V\210P\310o\231\271\036\301\022\032\205fC[|=\215\372\223\022 \311b\025?\323N\343\325\032\005\365\236\001XKj\004E\351\007\247\265fu\305j\367\331\275\253\307R\032 \014H#\014\272!#\345\306s\323\371\350\364\006.\000\356\230\353\270\263\215\217\303\256\220i^\277\305\214: \375\200zY\275\203}\375\244\205\035\340\226]l!uE\334\273\214\214\020\303\3474\360\014\234-\006\315B\031\022\010\022\006\010\001\022\002\010\000\032\r\022\013\n\007Org1MSP\020\001" >
    .
    .
    .
    2018-02-22 18:28:46.693 UTC [chaincodeCmd] upgrade -> DEBU 00c Get Signed envelope
    2018-02-22 18:28:46.693 UTC [chaincodeCmd] chaincodeUpgrade -> DEBU 00d Send signed envelope to orderer
    2018-02-22 18:28:46.908 UTC [main] main -> INFO 00e Exiting.....
    ```
