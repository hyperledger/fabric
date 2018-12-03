Upgrading to the Newest Version of Fabric
=========================================

At a high level, upgrading a Fabric network from v1.3 to v1.4 can be performed
by following these steps:

 * Upgrade the binaries for the ordering service, the Fabric CA, and the peers.
   These upgrades may be done in parallel.
 * Upgrade client SDKs.
 * (Optional) Upgrade the Kafka cluster.

To help understand this process, we've created the :doc:`upgrading_your_network_tutorial`
tutorial that will take you through most of the major upgrade steps, including
upgrading peers and orderers. We've included both a
script as well as the individual steps to achieve these upgrades.

Because our tutorial leverages the :doc:`build_network` (BYFN) sample, it has
certain limitations (it does not use Fabric CA, for example). Therefore we have
included a section at the end of the tutorial that will show how to upgrade
your CA, Kafka clusters, CouchDB, Zookeeper, vendored chaincode shims, and Node
SDK clients.

Because there are no new :doc:`capability_requirements` in v1.4, the upgrade
process does not require any channel configuration transactions.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
