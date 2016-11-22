

[v0.6-preview](https://github.com/hyperledger/fabric/tree/v0.6) September 16, 2016

A developer preview release of the Hyperledger Fabric intended
to exercise the release logistics and stabilize a set of capabilities for
developers to try out. This will be the last release under the original
architecture. All subsequent releases will deliver on the
[v1.0 architecture](https://github.com/hyperledger/fabric/blob/master/proposals/r1/Next-Consensus-Architecture-Proposal.md).

Key enhancements:

* 8de58ed - NodeSDK doc changes -- FAB-146
* 62d866d - Add flow control to SYNC_STATE_SNAPSHOT
* 4d97069 - Adding TLS changes to SDK
* e9d3ac2 - Node-SDK: add support for fabric events(block, chaincode, transactional)
* 7ed9533 - Allow deploying Java chaincode from remote git repositories
* 4bf9b93 - Move Docker-Compose files into their own folder
* ce9fcdc - Print ChaincodeName when deploy with CLI
* 4fa1360 - Upgrade go protobuf from 3-beta to 3
* 4b13232 - Table implementation in java shim with example
* df741bc - Add support for dynamically registering a user with attributes
* 4203ea8 - Check for duplicates when adding peers to the chain
* 518f3c9 - Update docker openjdk image
* 47053cd - Add GetTxID function to Stub interface (FAB-306)
* ac182fa - Remove deprecated devops REST API
* ad4645d - Support hyperledger fabric build on ppc64le platform
* 21a4a8a - SDK now properly adding a peer with an invalid URL
* 1d8114f - Fix setting of watermark on restore from crash
* a98c59a - Upgrade go protobuff from 3-beta to 3
* 937039c - DEVENV: Provide strong feedback when provisioning fails
* d74b1c5 - Make pbft broadcast timeout configurable
* 97ed71f - Java shim/chaincode project reorg, separate java docker env
* a76dd3d - Start container with HostConfig was deprecated since v1.10 and removed since v1.12
* 8b63a26 - Add ability to unregister for events
* 3f5b2fa - Add automatic peer command detection
* 6daedfd - Re-enable sending of chaincode events
* b39c93a - Update Cobra and pflag vendor libraries
* dad7a9d - Reassign port numbers to 7050-7060 range                                

[v0.5-developer-preview](https://github.com/hyperledger-archives/fabric/tree/v0.5-developer-preview)
June 17, 2016

A developer preview release of the Hyperledger Fabric intended
to exercise the release logistics and stabilize a set of capabilities for
developers to try out.

Key features:

Permissioned blockchain with immediate finality
Chaincode (aka smart contract) execution environments
Docker container (user chaincode)
In-process with peer (system chaincode)
Pluggable consensus with PBFT, NOOPS (development mode), SIEVE (prototype)
Event framework supports pre-defined and custom events
Client SDK (Node.js), basic REST APIs and CLIs
Known Key Bugs and work in progress

* 1895 - Client SDK interfaces may crash if wrong parameter specified
* 1901 - Slow response after a few hours of stress testing
* 1911 - Missing peer event listener on the client SDK
* 889  - The attributes in the TCert are not encrypted. This work is still on-going
