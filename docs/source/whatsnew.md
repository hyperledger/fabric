# What's new in v1.2

A quick rundown of the new features and documentation in the v1.2 release of
Hyperledger Fabric:

## New major features

* **[Private Data Collections](https://hyperledger-fabric.readthedocs.io/en/release-1.2/private-data/private-data.html)**:
  A way to keep certain data/transactions confidential among a subset of channel
  members. We also have an architecture document on this topic which can be found
  [here](https://hyperledger-fabric.readthedocs.io/en/release-1.2/private-data-arch.html).

* **[Service Discovery](https://hyperledger-fabric.readthedocs.io/en/release-1.2/discovery-overview.html)**:
  Discover network services dynamically, including orderers, peers, chaincode,
  and endorsement policies, to simplify client applications.

* **[Access control](https://hyperledger-fabric.readthedocs.io/en/release-1.2/access_control.html)**:
  How to configure which client identities can interact with peer functions on a
  per channel basis.

* **[Pluggable endorsement and validation](https://hyperledger-fabric.readthedocs.io/en/release-1.2/pluggable_endorsement_and_validation.html)**:
  Utilize pluggable endorsement and validation logic per chaincode.

### New tutorials

* **[Upgrade to version v1.2](https://hyperledger-fabric.readthedocs.io/en/release-1.2/upgrade_to_newest_version.html)**:
  Leverages the BYFN network to show how an upgrade flow should work. Includes
  both a script (which can serve as a template for upgrades), as well as the
  individual commands.

* **[CouchDB](https://hyperledger-fabric.readthedocs.io/en/release-1.2/couchdb_tutorial.html)**:
  How to set up a CouchDB data store (which allows for rich queries).

* **[Private data](https://hyperledger-fabric.readthedocs.io/en/release-1.2/private_data_tutorial.html)**:
  Shows how to set up a collection using BYFN.

* **[Query certificates based on various filter criteria (Fabric CA)](https://hyperledger-fabric-ca.readthedocs.io/en/latest/users-guide.html#manage-certificates)**:
  Describes how to use `fabric-ca-client` to manage certificates.

### New conceptual documentation

* **[Fabric network as a concept](https://hyperledger-fabric.readthedocs.io/en/release-1.2/network/network.html)**:
  A look at the structure of the various pieces of a Fabric network and how they
  interact.

* **Private data**: See above.

### Other new documentation

* **[Service Discovery CLI](https://hyperledger-fabric.readthedocs.io/en/release-1.2/discovery-cli.html)**:
  Configuring the discovery service using the CLI.

## Release notes

For more information, including `FAB` numbers for the issues and code reviews
that made up these changes (in addition to other hygiene/performance/bug fixes
we did not explicitly document), check out the:

* [Fabric release notes](https://github.com/hyperledger/fabric/releases/tag/v1.2.0).
* [Fabric CA release notes](https://github.com/hyperledger/fabric-ca/releases/tag/v1.2.0).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
