Upgrading to the latest release
===============================

If you’re familiar with previous releases of Hyperledger Fabric, you’re aware
that upgrading the nodes and channels to the latest version of Fabric is, at a
high level, a four step process.

1. Backup the ledger and MSPs.
2. Upgrade the orderer binaries in a rolling fashion to the latest Fabric version.
3. Upgrade the peer binaries in a rolling fashion to the latest Fabric version.
4. Update application channels to the latest
   capability levels, where available. Note that some releases will have
   capabilities in all groups while other releases may have few or even no new
   capabilities at all.

For more information about capabilities, check out :doc:`capabilities_concept`.

For a look at how these upgrade processes are accomplished, please consult these
tutorials:

1. :doc:`upgrade_to_newest_version`. This topic discusses the important considerations
   for getting to the latest release.
2. :doc:`upgrading_your_components`. Components should be upgraded to the latest
   version before updating any capabilities.
3. :doc:`updating_capabilities`. Completed after updating the versions of all nodes.

.. note:: SDK applications can be upgraded separate from a general upgrade of your Fabric network.
          The `Fabric Gateway client API <https://github.com/hyperledger/fabric-gateway>`_ has been tested with Fabric v2.5 and v3.0.
          If you have not yet migrated to the Fabric Gateway client API,
          you can `migrate <https://hyperledger.github.io/fabric-gateway/migration/>`_ while using a Fabric v2.5 network,
          or after you have upgraded to a Fabric v3.0 network. The legacy SDKs are no longer maintained and are not compatible with new v3.0 Fabric features such as SmartBFT consensus.

.. toctree::
   :maxdepth: 1
   :caption: Upgrading to the latest release

   upgrade_to_newest_version
   upgrading_your_components
   updating_capabilities
   enable_cc_lifecycle
