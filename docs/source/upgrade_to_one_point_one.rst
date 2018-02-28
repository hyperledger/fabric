Upgrading from v1.0.x
=====================

At a high level, upgrading a Fabric network to v1.1 can be performed by following these
steps:

 * Upgrade binaries for orderers, peers, and fabric-ca. These upgrades may be done in parallel.
 * Upgrade client SDKs.
 * Enable v1.1 channel capability requirements.
 * (Optional) Upgrade the Kafka cluster.

To help understand this process, we've created the :doc:`upgrading_your_network_tutorial`
tutorial that will take you through most of the major upgrade steps, including
upgrading peers, orderers, as well as enabling capability requirements.

Because our tutorial leverages the :doc:`build_network` (BYFN) sample, it has
certain limitations (it does not use Fabric CA, for example). Therefore we have
included a section at the end of the tutorial that will show how to upgrade
your CA, Kafka clusters, CouchDB, Zookeeper, vendored chaincode shims, and Node
SDK clients.

If you want to learn more about capability requirements, click `here <http://hyperledger-fabric.readthedocs.io/en/latest/capability_requirements.html>`_.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
