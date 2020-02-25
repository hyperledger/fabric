What's new in v1.4
==================

Hyperledger Fabric's first long term support release
----------------------------------------------------

Hyperledger Fabric has matured since the initial v1.0 release, and so has the
community of Fabric operators. The Fabric developers have been working with
network operators to deliver v1.4 with a focus on stability and production
operations. As such, v1.4.x will be our first long term support release.

Our policy to date has been to provide bug fix (patch) releases for our most
recent major or minor release until the next major or minor release has been
published. We plan to continue this policy for subsequent releases. However,
for Hyperledger Fabric v1.4, the Fabric maintainers are pledging to provide
bug fixes for a period of one year from the date of release. This will likely
result in a series of patch releases (v1.4.1, v1.4.2, and so on), where multiple
fixes are bundled into a patch release.

If you are running with Hyperledger Fabric v1.4.x, you can be assured that
you will be able to safely upgrade to any of the subsequent patch releases.
In the advent that there is need of some upgrade process to remedy a defect,
we will provide that process with the patch release.

Raft ordering service
---------------------

Introduced in v1.4.1, `Raft <https://raft.github.io/raft.pdf>`_ is a crash fault
tolerant (CFT) ordering service based on an implementation of Raft protocol in
`etcd <https://coreos.com/etcd/>`_. Raft follows a "leader and follower" model,
where a leader node is elected (per channel) and its decisions are replicated to
the followers. Raft ordering services should be easier to set up and manage than
Kafka-based ordering services, and their design allows organizations spread out
across the world to contribute nodes to a decentralized ordering service.

* :doc:`orderer/ordering_service`:
  Describes the role of an ordering service in Fabric and an overview of the
  three ordering service implementations currently available: Solo, Kafka, and
  Raft.

* :doc:`raft_configuration`:
  Shows the configuration parameters and considerations when deploying a Raft
  ordering service.

* :doc:`orderer_deploy`:
  Describes the process for deploying an ordering node, independent of what the
  ordering service implementation will be.

* :doc:`build_network`:
  The ability to stand up a sample network using a Raft ordering service has been
  added to this tutorial.

* :doc:`kafka_raft_migration`:
  If you're a user with a Kafka ordering service, this doc shows the process for
  migrating to a Raft ordering service. Available since v1.4.2.

Serviceability and operations improvements
------------------------------------------

As more Hyperledger Fabric networks enter a production state, serviceability and
operational aspects are critical. Fabric v1.4 takes a giant leap forward with
logging improvements, health checks, and operational metrics. As such, Fabric v1.4
is the recommended release for production operations.

* :doc:`operations_service`:
  The new RESTful operations service provides operators with three
  services to monitor and manage peer and orderer node operations:

  * The logging ``/logspec`` endpoint allows operators to dynamically get and set
    logging levels for the peer and orderer nodes.

  * The ``/healthz`` endpoint allows operators and container orchestration services to
    check peer and orderer node liveness and health.

  * The ``/metrics`` endpoint allows operators to utilize Prometheus to pull operational
    metrics from peer and orderer nodes. Metrics can also be pushed to StatsD.

  * As of v1.4.4, the ``/version`` endpoint allows operators to query the release version
    of the peer and orderer and the commit SHA from which the release was cut.

Improved programming model for developing applications
------------------------------------------------------

Writing decentralized applications has just gotten easier. Programming model
improvements for smart contracts (chaincode) and the SDKs makes the development
of decentralized applications more intuitive, allowing you to focus
on your application logic. These programming model enhancements are available
for Node.js (as of Fabric v1.4.0) and Java (as of Fabric v1.4.2). The existing
SDKs are still available for use and existing applications will continue to work.
It is recommended that developers migrate to the new SDKs, which provide a layer
of abstraction to improve developer productivity and ease of use.

New documentation helps you
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
  This tutorial has been updated to leverage the improved smart contract (chaincode)
  and SDK programming model. The tutorial has Java, JavaScript, and Typescript examples
  of the client application and chaincode.

* :doc:`tutorial/commercial_paper`
  As mentioned above, this is the tutorial that accompanies the new Developing
  Applications documentation. This contains both Java and JavaScript code.

* :doc:`upgrade_to_newest_version`:
  Leverages the network from :doc:`build_network` to demonstrate an upgrade from
  v1.3 to v1.4.x. Includes both a script (which can serve as a template for upgrades),
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

Node OU support
---------------

* :doc:`msp`:
  Starting with v1.4.3, node OUs are now supported for admin and orderer identity
  classifications (extending the existing Node OU support for clients and peers).
  These "organizational units" allow organizations to further classify identities
  into admins and orderers based on the OUs of their x509 certificates.

Release notes
=============

The release notes provide more details for users moving to the new release, along
with a link to the full release change log.

* `Fabric v1.4.0 release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.0>`_.
* `Fabric v1.4.1 release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.1>`_.
* `Fabric v1.4.2 release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.2>`_.
* `Fabric v1.4.3 release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.3>`_.
* `Fabric v1.4.4 release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.4>`_.
* `Fabric v1.4.5 release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.5>`_.
* `Fabric v1.4.6 release notes <https://github.com/hyperledger/fabric/releases/tag/v1.4.6>`_.
* `Fabric CA v1.4.0 release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.0>`_.
* `Fabric CA v1.4.1 release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.1>`_.
* `Fabric CA v1.4.2 release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.2>`_.
* `Fabric CA v1.4.3 release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.3>`_.
* `Fabric CA v1.4.4 release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.4>`_.
* `Fabric CA v1.4.5 release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.5>`_.
* `Fabric CA v1.4.6 release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v1.4.6>`_.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
