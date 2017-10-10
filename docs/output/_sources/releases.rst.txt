Release Notes
=============

`v1.0.0-alpha <https://github.com/hyperledger/fabric/releases/tag/v1.0.0-alpha>`__
March 16, 2017

Key enhancements:

- 3b6c70d FAB-1128 further cleanup of protos
- 3ac1bd3 FAB-1129 Add cc return value to proposal response
- b95adc8 Introduce two new message for gossip proto
- 9bd29d3 Add tests to static bootstrap helper
- 2f7153f BCCSP ECDSA/RSA/X509 public/private key import
- cb9a29b [FAB-996] Introduce orderer/commons/util package
- b3b4e54 [FAB-773] gossip state transfer, block re-ordering
- 742443e FAB-872 Multichannel support: message extension
- 5f98d54 Integration of MSP in endorser
- 60706a7 [FAB-1094] util to parse config tx blocks
- 445fbdb Added support for advance KV-queries
- 4a6b894 Change how chaintool executes
- c1e6fb4 [FAB-1161] Push genesis block upon orderer init
- 55fd4c4 BCCSP Generalized Key Import
- 5f08c25 Gossip integration auxilary
- 21a4c6a FAB-1128 finalize protos - remove discovery and devops
- 73c501c [FAB-798] Abstract out the solo broadcast handler
- 1b5d378 [FAB-798] Abstract out the solo deliver handler
- b7908a3 [FAB-798] Factor common gRPC components from solo
- a2b9b2e [FAB-798] Factor out block cutting logic
- 6b58537 [FAB-421] Add multi-chain support to rawledger
- 68aef4e Removing primitives package dependency from BCCSP
- 16fa08e TX proposal/endorsement/validation flow (+MSP)
- f6d1be2 [FAB-1190] Make Rawledger accept metadata
- 2013daa BCCSP KeyStore
- 2830cfb [FAB-884] implement basic query cli
- b1ecf80 FAB-1192 timer should be reset each pop
- a8af1e9 Hook multichain manager into main path
- 28f16aa [FAB-931] Add multi-broker Kafka orderer environments
- 6856308 Suppress logging output of the peer during unit-tests
- e9f9806 Remove rocksdb dependency
- 3731447 FAB-1087 Add config option in core.yaml for history
- 80140c9 Allow deploying Java chaincode from remote git repo
- eba912b Add interactive asset management demo
- 9baa4eb Add common CLI function to get a server admin client
- 0183483 FAB-1291: Couch support for doing a savepoint.
- 96637cf Rework of MSP (config and factories)
- ebb3cb9 Enable block event generation
- 9662335 Ledger API to retrieve last block
- 1f4b004 Refactor MSP package and msp config w/o json
- f97b321 FAB-1020 Configuration system chaincode
- b6ab3f8 Upgrade to golang 1.7 in travis ci
- 8417c0e [FAB-1288]: Expose gossip API for cscc.
- 2ebd342 FAB-1172 - Advanced simulation functions for CouchDB
- 746b873 [FAB-814] Introduce ChainCreators orderer config
- d18aa98 FAB-1140 Ledger History Database framework
- 783e7d0 FAB-1020 Configuration system chaincode
- 458c521 FAB-1336 Add new ledger blockstorage index.
- 6444545 MSP mgr instantiation from Block
- 79aa4df [FAB-1384]: Change ValidatedLedger APIs
- 0b162ca PKCS11/MSH compatible BCCSP SKI gen
- 0567b34 FAB-1395 - Generic query API for CouchDB
- 6c45ffa FAB-1259 Create Basic Common GRPC Server
- 8d53e6d FAB-1018 MultiChannel API fabric<-->gossip
- d39194c Added support for TLS in java shim
- a9ae6e7 Upgrade golang 1.6 to 1.7 in chaincode examples
- 2ae4ed3 [FAB-872] Gossip multi-channel: channel
- fdf2f7a [FAB-872] Gossip multiChannel support
- 7b0aef8 FAB-1257 Removal of Table API
- 457bb90 FAB-1166 Gossip leader election
- c14896a Ledger query APIs
- 4b2947c [FAB-1500] Recovery of history database
- d58d51b [FAB-1390] Refactor ledger interface names
- 0377199 [FAB-187] - using policies in VSCC
- c65e40e FAB-829: App library for access control/App. MSP
- a0898e6 [FAB-1648] Install SoftHSM for testing PKCS11 CSP
- 4916ac4 [FAB-1648] Vendor PKCS11 bindings
- d5467f3 [FAB-204] Expose ledger rich query API to chaincode
- 0a94993 [FAB-1858] Provide gossip with channel config
- 9ca80f1 [FAB-1885] GetTransactionByID to return Tran Envelope
- 1bd5b2b [FAB-1790, FAB-1791] Chaincode calling chaincode
- 25c888d [FAB-1700] Determinsitic BlockData hashing
- 4c9bec7 [FAB-1809] Enable tls config for Kafka connection
- d1e939f [FAB-1956] Automatically generate orderer template
- c0ce696 [FAB-2125] Finalize v1 chaincode API names
- 32b772c [FAB-2169] Dynamically generate genesis material
- ca02c60 [FAB-2122] Scan codepackage for illegal content
- a971b0f [FAB-2319] Implement hierarchical policies storage
- 8e2563d Use a minimal container for GOLANG/CAR chaincode
- 29954d6 Orderer Traffic Engine (OTE) FAB-1805
- 458328b Chaincode API Enhancement
- 589efc6 [FAB-1558] - Revocation support in MSP
- cec4b5c Replace Shake with SHA
- 2fc6bc6 [FAB-2080] - peer enforces ACLs on proposals
- 3e0481b [FAB-2087] - support for admin policy principals
- 9fe8c60 [FAB-1934] admin validation for chain-scoped syscc
- 4013cb6 [FAB-2432] Encode anchor peers from configtx.yaml
- 81cd41b FAB-1438: Add up, down, scale to compose util
- ff8b3e4 [FAB-2206]Make gossip discovery configurable
- fc62148 FAB2044: Allow OUs to be contained in MSP description
- 306aa7d Add query to get instantiated chaincodes on a channel
- 151a9a6 Converge deployment spec validation
- f8a8ddd Upgrade to chaincode v0.10.3
- b7b5c4e [FAB-2198] Gossip envelope refactoring
- 4246971 Prevent CLI to connect to ordering service on join.
- be91ccc [FAB-2545] Add tool to create various crypto configs
- 29ea124 Change project status from Incubation to Active (again)
- 5f4b99a [FAB-2503] CLI based End-to-End flow test verification
- 5ca0611 Add ability to customize chaincode container log format
- ba68129 FAB-2671 e2e_cli to use OrdererMSP consistently
- 6a81ec1 [FAB-2632] Default endorsement policy
- 88cb6cc [FAB-2691] Improve Bcst/Dlvr log serviceability
- 1f49bfb [FAB-2714] Enable peer to start with TLS enabled
- 3ad3e43 [FAB-2710] Gossip: Log WARN upon bad network config
- a3e3940 [FAB-2696] Default chain broken in peer
- 124cd2d [FAB-1141] Updating TLS and gossip leader conf
- 626fcd3 Add Channel information to block-listener
- 3169234 FAB-2081 allow user CC to call system CC
- f8a49c0 [FAB-2745] Update e2e_cli to work with TLS
- 844fe2d [FAB-2773] Restrict the total count of channels
- 0308f0f [FAB-1141] Enabling TLS in bootstrap feature
- 0f38dc1 [FAB-2565] Example docker-compose with CouchDB
- fa3d88c Release 1.0.0-alpha

`v0.6-preview <https://github.com/hyperledger/fabric/tree/v0.6>`__
September 16, 2016

A developer preview release of the Hyperledger Fabric intended to
exercise the release logistics and stabilize a set of capabilities for
developers to try out. This will be the last release under the original
architecture. All subsequent releases will deliver on the `v1.0
architecture <TODO>`__.

Key enhancements:

-  8de58ed - NodeSDK doc changes -- FAB-146
-  62d866d - Add flow control to SYNC\_STATE\_SNAPSHOT
-  4d97069 - Adding TLS changes to SDK
-  e9d3ac2 - Node-SDK: add support for fabric events(block, chaincode,
   transactional)
-  7ed9533 - Allow deploying Java chaincode from remote git repositories
-  4bf9b93 - Move Docker-Compose files into their own folder
-  ce9fcdc - Print ChaincodeName when deploy with CLI
-  4fa1360 - Upgrade go protobuf from 3-beta to 3
-  4b13232 - Table implementation in java shim with example
-  df741bc - Add support for dynamically registering a user with
   attributes
-  4203ea8 - Check for duplicates when adding peers to the chain
-  518f3c9 - Update docker openjdk image
-  47053cd - Add GetTxID function to Stub interface (FAB-306)
-  ac182fa - Remove deprecated devops REST API
-  ad4645d - Support hyperledger fabric build on ppc64le platform
-  21a4a8a - SDK now properly adding a peer with an invalid URL
-  1d8114f - Fix setting of watermark on restore from crash
-  a98c59a - Upgrade go protobuff from 3-beta to 3
-  937039c - DEVENV: Provide strong feedback when provisioning fails
-  d74b1c5 - Make pbft broadcast timeout configurable
-  97ed71f - Java shim/chaincode project reorg, separate java docker env
-  a76dd3d - Start container with HostConfig was deprecated since v1.10
   and removed since v1.12
-  8b63a26 - Add ability to unregister for events
-  3f5b2fa - Add automatic peer command detection
-  6daedfd - Re-enable sending of chaincode events
-  b39c93a - Update Cobra and pflag vendor libraries
-  dad7a9d - Reassign port numbers to 7050-7060 range

`v0.5-developer-preview <https://github.com/hyperledger-archives/fabric/tree/v0.5-developer-preview>`__
June 17, 2016

A developer preview release of the Hyperledger Fabric intended to
exercise the release logistics and stabilize a set of capabilities for
developers to try out.

Key features:

Permissioned blockchain with immediate finality Chaincode (aka smart
contract) execution environments Docker container (user chaincode)
In-process with peer (system chaincode) Pluggable consensus with PBFT,
NOOPS (development mode), SIEVE (prototype) Event framework supports
pre-defined and custom events Client SDK (Node.js), basic REST APIs and
CLIs Known Key Bugs and work in progress

-  1895 - Client SDK interfaces may crash if wrong parameter specified
-  1901 - Slow response after a few hours of stress testing
-  1911 - Missing peer event listener on the client SDK
-  889 - The attributes in the TCert are not encrypted. This work is
   still on-going
