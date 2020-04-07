Upgrading to the latest release
===============================

If you’re familiar with previous releases of Hyperledger Fabric, you’re aware
that upgrading the nodes and channels to the latest version of Fabric is, at a
high level, a four step process.

1. Backup the ledger and MSPs.
2. Upgrade the orderer binaries in a rolling fashion to the latest Fabric version.
3. Upgrade the peer binaries in a rolling fashion to the latest Fabric version.
4. Update the orderer system channel and any application channels to the latest
   capability levels, where available. Note that some releases will have
   capabilities in all groups while other releases may have few or even no new
   capabilities at all.

For more information about capabilities, check out :doc:`capabilities_concept`.

For a look at how these upgrade processes are accomplished, please consult these
tutorials:

1. :doc:`upgrade_to_newest_version`. This topic discusses the important considerations
   for getting to the latest release from the previous release as well as from
   the most recent long term support (LTS) release.
2. :doc:`upgrading_your_components`. Components should be upgraded to the latest
   version before updating any capabilities.
3. :doc:`updating_capabilities`. Completed after updating the versions of all nodes.
4. :doc:`enable_cc_lifecycle`. Necessary to add organization specific endorsement
   policies central to the new chaincode lifecycle for Fabric v2.x.

As the upgrading of nodes and increasing the capability levels of channels is by
now considered a standard Fabric process, we will not show the specific commands
for upgrading to the newest release. Similarly, there is no script in the ``fabric-samples``
repo that will upgrade a sample network from the previous release to this one,
as there has been for previous releases.

.. note:: It is a best practice to upgrade your SDK to the latest version as a
          part of a general upgrade of your network. While the SDK will always
          be compatible with equivalent releases of Fabric and lower, it might
          be necessary to upgrade to the latest SDK to leverage the latest Fabric
          features. Consult the documentation of the Fabric SDK you are using
          for information about how to upgrade.

.. toctree::
   :maxdepth: 1
   :caption: Upgrading to the latest release

   upgrade_to_newest_version
   upgrading_your_components
   updating_capabilities
   enable_cc_lifecycle
