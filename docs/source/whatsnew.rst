What's new in v1.4
==================

Hyperledger Fabric has matured since the initial v1.0 release, and so has the
community of Fabric operators and developers. The Fabric developers have been
working with network operators and application developers to deliver v1.4 with
a focus on production operations and developer ease of use. The two major
release themes for Hyperledger Fabric v1.4 revolve around these two areas:

* **Serviceability and Operations**: As more Hyperledger Fabric networks get
  deployed and enter a production state, serviceability and operational aspects
  are critical. Fabric v1.4 takes a giant leap forward with logging improvements,
  health checks, and operational metrics. Along with a focus on stability
  and fixes, Fabric v1.4 is the recommended release for production operations.
  Future fixes will be delivered on the v1.4.x stream, while new features are
  being developed in the v2.0 stream.

* **Improved programming model for developing applications**: Writing
  decentralized applications has just gotten easier. Programming model
  improvements in the Node.js SDK and Node.js chaincode makes the development
  of decentralized applications more intuitive, allowing you to focus
  on your application logic. The existing npm packages are still available for
  use, while the new npm packages provide a layer of abstraction to improve
  developer productivity and ease of use.

Serviceability and operations improvements
------------------------------------------

* :doc:`operations_service`:
  The new RESTful operations service provides operators with three
  services to monitor and manage peer and orderer node operations:

  * The logging ``/logspec`` endpoint allows operators to dynamically get and set
    logging levels for the peer and orderer nodes.

  * The ``/healthz`` endpoint allows operators and container orchestration services to
    check peer and orderer node liveness and health.

  * The ``/metrics`` endpoint allows operators to utilize Prometheus to pull operational
    metrics from peer and orderer nodes. Metrics can also be pushed to StatsD.

Improved programming model for developing applications
------------------------------------------------------

The new Node.js SDK and chaincode programming model makes developing decentralized
applications easier and improves developer productivity. New documentation helps you
understand the various aspects of creating a decentralized application for
Hyperledger Fabric, using a commercial paper business network scenario.

* :doc:`developapps/scenario`:
  Describes a hypothetical business network involving six organizations who want
  to build an application to transact together that will serve as a use case
  to describe the programming model.

* :doc:`developapps/analysis`:
  Describes the structure of a commercial paper and how transactions affect it
  over time. Demonstrates that modeling using states and transactions
  provides a precise way to understand and model the decentralized business process.

* :doc:`developapps/architecture`:
  Shows how to design the commercial paper processes and their related data
  structures.

* :doc:`developapps/smartcontract`:
  Shows how a smart contract governing the decentralized business process of
  issuing, buying and redeeming commercial paper should be designed.

* :doc:`developapps/application`
  Conceptually describes a client application that would leverage the smart contract
  described in :doc:`developapps/smartcontract`.

* :doc:`developapps/designelements`:
  Describes the details around contract namespaces, transaction context,
  transaction handlers, connection profiles, connection options, wallets, and
  gateways.

And finally, a tutorial and sample that brings the commercial paper scenario to life:

* :doc:`tutorial/commercial_paper`

New tutorials
-------------

* :doc:`write_first_app`:
  This tutorial has been updated to leverage the improved Node.js SDK and chaincode
  programming model. The tutorial has both JavaScript and Typescript examples of
  the client application and chaincode.

* :doc:`tutorial/commercial_paper`
  As mentioned above, this is the tutorial that accompanies the new Developing
  Applications documentation.

* :doc:`upgrade_to_newest_version`:
  Leverages the network from :doc:`build_network` to demonstrate an upgrade from
  v1.3 to v1.4. Includes both a script (which can serve as a template for upgrades),
  as well as the individual commands so that you can understand every step of an
  upgrade.

Private data enhancements
-------------------------

* :doc:`private-data-arch`:
  The Private data feature has been a part of Fabric since v1.2, and this release
  debuts two new enhancements:

  * **Reconciliation**, which allows peers for organizations that are added
    to private data collections to retrieve the private data for prior
    transactions to which they now are entitled.

  * **Client access control** to automatically enforce access control within
    chaincode based on the client organization collection membership without having
    to write specific chaincode logic.

Release notes
=============

The release notes provide more details for users moving to the new release, along
with a link to the full release change log.

* `Fabric release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.0-rc2>`_.
* `Fabric CA release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.0-rc2>`_.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
