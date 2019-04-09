What's new in the v2.0 Alpha
============================

A word about the Alpha release
------------------------------

The Alpha release of Hyperledger Fabric v2.0 allows users to try out two exciting
new features --- the new Fabric chaincode lifecycle and FabToken. The Alpha release
is being offered to provide users a preview of new capabilities and is not meant
to be used in production. Additionally there is no upgrade support to the v2.0
Alpha release, and no intended upgrade support from the the Alpha release
to future versions of v2.x.

Fabric chaincode lifecycle
--------------------------

The Fabric 2.0 Alpha introduces decentralized governance for chaincode, with
a new process for installing a chaincode on your peers and starting it
on a channel. The new Fabric chaincode lifecycle allows
multiple organizations to come to agreement on the parameters of a chaincode,
such as the chaincode endorsement policy, before it can be used to interact
with the ledger. The new model offers several improvements over the previous
lifecycle:

* **Multiple organizations must agree to the parameters of a chaincode:** In
  the release 1.x versions of Fabric, one organization had the ability to set
  parameters of a chaincode (for instance the endorsement policy) for all other
  channel members. The new Fabric chaincode lifecycle is more flexible since
  it supports both centralized trust models (such as that of the previous
  lifecycle model) as well as decentralized models requiring a sufficient number
  of organizations to agree on an endorsement policy before it goes into effect.

* **Safer chaincode upgrade process:** In the previous chaincode lifecycle,
  the upgrade transaction could be issued by a single organization, creating a
  risk for a channel member that had not yet installed the new chaincode. The
  new model allows for a chaincode to be upgraded only after a sufficient
  number of organizations have approved the upgrade.

* **Easier endorsement policy updates:** Fabric lifecycle allows you to change
  an endorsement policy without having to repackage or reinstall the chaincode.
  Users can also take advantage of a new default policy that requires endorsement
  from a majority of members on the channel. This policy is updated automatically
  when organizations are added or removed from the channel.

* **Inspectable chaincode packages:** The Fabric lifecycle packages chaincode in
  easily readable tar files. This makes it easier to inspect the chaincode
  package and coordinate installation across multiple organizations.

* **Start multiple chaincodes on a channel using one package:** The previous
  lifecycle defined each chaincode on the channel using a name and version that
  was specified when the chaincode package was installed. You can now use a
  single chaincode package and deploy it multiple times with different names
  on the same or different channel.

Using the new chaincode lifecycle
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Use the following tutorials to get started with the new chaincode lifecycle:

* :doc:`chaincode4noah`:
  Provides a detailed overview of the steps required to install and define a
  chaincode, as well as the capabilities available with the new model.

* :doc:`build_network`:
  If you want to start using the new lifecycle right away, the BYFN tutorial has
  been updated to use the :doc:`commands/peerlifecycle` CLI to install and
  define chaincode on a sample network.

* :doc:`private_data_tutorial`:
  Has been updated to demonstrate how to use :doc:`private-data/private-data`
  collections with the new chaincode lifecycle.

* :doc:`endorsement-policies`:
  Learn how the new lifecycle allows you to use policies in the channel
  configuration as chaincode endorsement policies.

Restrictions and limitations
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The new Fabric chaincode lifecycle in the v2.0 Alpha release is not yet feature
complete. Specifically, be aware of the following limitations in the Alpha release:

- CouchDB indexes are not yet supported
- Chaincodes defined with the new lifecycle are not yet discoverable via service
  discovery

These limitations will be resolved after the Alpha release.

FabToken
--------

The Fabric 2.0 Alpha also provides users the ability to easily represent
assets as tokens on Fabric channels. FabToken is a token management system that
uses an Unspent Transaction Output (UTXO) model to issue, transfer, and redeem
tokens using the identity and membership infrastructure provided by Hyperledger
Fabric.

* :doc:`token/FabToken`:
  This operations guide provides a detailed overview of how to use tokens on a
  Fabric network. The guide also includes an example on how to create and
  transfer tokens using the :doc:`commands/token` CLI.

Alpine images
-------------

Starting with v2.0, Hyperledger Fabric Docker images will use Alpine Linux, a
security-oriented, lightweight Linux distribution. This means that Docker images
are now much smaller, providing faster download and startup
times, as well as taking up less disk space on host systems. Alpine Linux
is designed from the ground up with security in mind, and the
minimalist nature of the Alpine distribution greatly reduces the risk of
security vulnerabilities.

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
  Has been updated to allow you to use a Raft ordering service with a sample
  network.

Release notes
=============

The release notes provide more details for users moving to the new release, along
with a link to the full release change log.

* `Fabric v2.0.0-alpha release notes <https://github.com/hyperledger/fabric/releases/tag/v2.0.0-alpha>`_.
* `Fabric CA v2.0.0-alpha release notes <https://github.com/hyperledger/fabric-ca/releases/tag/v2.0.0-alpha>`_.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
