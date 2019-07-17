Upgrading to the Newest Version of Fabric
=========================================

At a high level, upgrading a Fabric network from v1.3 to v1.4 can be performed
by following these steps:

 * Upgrade the binaries for the ordering service, the Fabric CA, and the peers.
   These upgrades may be done in parallel.
 * Upgrade client SDKs.
 * If upgrading to v1.4.2, enable the v1.4.2 channel capabilities.
 * (Optional) Upgrade the Kafka cluster.

To help understand this process, we've created the :doc:`upgrading_your_network_tutorial`
tutorial that will take you through most of the major upgrade steps, including
upgrading peers, orderers, as well as the capabilities. We've included both a
script as well as the individual steps to achieve these upgrades.

Because our tutorial leverages the :doc:`build_network` (BYFN) sample, it has
certain limitations (it does not use Fabric CA, for example). Therefore we have
included a section at the end of the tutorial that will show how to upgrade
your CA, Kafka clusters, CouchDB, Zookeeper, vendored chaincode shims, and Node
SDK clients.

While upgrade to v1.4.0 does not require any capabilities to be enabled,
v1.4.2 offers new capabilities at the orderer, channel, and application levels.
Specifically, the v1.4.2 capabilities enable the following features:

 * Migration from Kafka to Raft consensus (requires v1.4.2 orderer and channel capabilities)
 * Ability to specify orderer endpoints per organization (requires v1.4.2 channel capability)
 * Ability to store private data for invalidated transactions (requires v1.4.2 application capability)

If you want to learn more about capability requirements, check out the
:doc:`capability_requirements` documentation.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
