# peer lifecycle chaincode

The `peer lifecycle chaincode` subcommand allows administrators to use the
Fabric chaincode lifecycle to package a chaincode, install it on your peers,
approve a chaincode definition for your organization, and then commit the
definition to a channel. The chaincode is ready to be used after the definition
has been successfully committed to the channel. For more information, visit
[Fabric chaincode lifecycle](../chaincode_lifecycle.html).

*Note: These instructions use the Fabric chaincode lifecycle introduced in the
v2.0 release. If you would like to use the old lifecycle to install and
instantiate a chaincode, visit the [peer chaincode](peerchaincode.html) command
reference.*

## Syntax

The `peer lifecycle chaincode` command has the following subcommands:

  * package
  * install
  * queryinstalled
  * getinstalledpackage
  * calculatepackageid
  * approveformyorg
  * queryapproved
  * checkcommitreadiness
  * commit
  * querycommitted

Each peer lifecycle chaincode subcommand is described together with its options in its own
section in this topic.
