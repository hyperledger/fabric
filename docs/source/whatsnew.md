# What's new in v1.3

A quick rundown of the new features and documentation in the v1.3 release of
Hyperledger Fabric:

## New features

* **[Identity Mixer](https://hyperledger-fabric.readthedocs.io/en/release-1.3/idemix.html)**:
  A way to keep identities anonymous and unlinkable through the use of zero-knowledge
  proofs. There is a tool that can generate Identity Mixer credentials in test
  environments known as `idexmigen`, the documentation for which can be found [here](https://hyperledger-fabric.readthedocs.io/en/release-1.3/idemixgen.html).

* **[State-based endorsement](https://hyperledger-fabric.readthedocs.io/en/release-1.3/endorsement-policies.html)**:
  Allows the default chaincode-level endorsement policy to be overridden by a
  per-key endorsement policy.

* **[CouchDB Pagination](https://hyperledger-fabric.readthedocs.io/en/release-1.3/couchdb_as_state_database.html#couchdb-pagination)**:
  The "paging" of results (only allowing a configurable number of results per
  page) allows clients to page through large result sets. This improves
  performance, which is particularly important in active networks with large
  numbers of transactions.

* **[Java chaincode support](https://hyperledger-fabric.readthedocs.io/en/release-1.3/chaincode4ade.html)**:
  As an addition to the current Fabric support for chaincode written in Go and
  node.js, Java is now supported. You can find a javadoc for this [here](https://fabric-chaincode-java.github.io/).

* **[Event hub removal](https://hyperledger-fabric.readthedocs.io/en/release-1.3/peer_event_services.html)**:
  The peer-channel event hub service itself is not new (it first debuted in v1.1),
  but the v1.3 release marks the end of the old event hub. Applications using
  the old event hub must switch over to the new peer-channel event hub prior to
  upgrading to v1.3.

### New tutorials

* **[Upgrade to version v1.3](https://hyperledger-fabric.readthedocs.io/en/release-1.3/upgrade_to_newest_version.html)**:
  Leverages the BYFN network to show how an upgrade flow should work. Includes
  both a script (which can serve as a template for upgrades), as well as the
  individual commands.

* **[CouchDB Pagination](https://hyperledger-fabric.readthedocs.io/en/release-1.3/couchdb_tutorial.html#cdb-pagination)**:
  Expands the current CouchDB tutorial to add pagination.

### Other new documentation

* **[Fabric network as a concept (edited and expanded)](https://hyperledger-fabric.readthedocs.io/en/release-1.3/network/network.html)**:
  Conceptual documentation that shows how the parts of a network interact with
  each other. The initial version of this document was added in v1.2.

## Release notes

For more information, including `FAB` numbers for the issues and code reviews
that made up these changes (in addition to other hygiene/performance/bug fixes
we did not explicitly document), check out the release notes. Note that these
links will not work on the release candidate, only on the GA release.

* [Fabric release notes](https://github.com/hyperledger/fabric/releases/tag/v1.3.0).
* [Fabric CA release notes](https://github.com/hyperledger/fabric-ca/releases/tag/v1.3.0).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
