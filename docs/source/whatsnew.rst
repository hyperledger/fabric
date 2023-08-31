What's new in Hyperledger Fabric
================================

What's New in Hyperledger Fabric v3.0
-------------------------------------

Byzantine Fault Tolerant (BFT) ordering service
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Hyperledger Fabric has utilized a Raft crash fault tolerant (CFT) ordering service since version v1.4.
A Byzantine Fault Tolerant (BFT) ordering service can withstand not only crash failures, but also a subset of nodes behaving maliciously.
Fabric v3.0 is the first release to provide a BFT ordering service based on the
`SmartBFT <https://arxiv.org/abs/2107.06922>`_ `consensus library <https://github.com/SmartBFT-Go/consensus>`_.
Consider using the BFT orderer if true decentralization is required,
where up to and not including a third of the parties running the orderers may not be trusted due to malicious intent or being compromised.

Learn more about the BFT ordering service in the **BFT** section of the :doc:`orderer/ordering_service` documentation.

For more details see the :doc:`bft_configuration` topic.

Try it out for yourself! Look for the **Bring up the network using BFT ordering service** in the :doc:`test_network` tutorial.
Alternatively, you can run the binaries locally by following the steps in the `test-network-nano-bash sample <https://github.com/hyperledger/fabric-samples/tree/main/test-network-nano-bash>`_.

Hyperledger Fabric v2.5
-----------------------

Hyperledger Fabric v2.5 remains the long-term support (LTS) release for production users.
Check out the `What's New in v2.5 topic <https://hyperledger-fabric.readthedocs.io/en/release-2.5/whatsnew.html>`_
to learn about recent features since the prior v2.2 LTS release, including
the ability to manage channels without a system channel,
the ability to take ledger snapshots and join peers to a channel based on a snapshot,
and the new Fabric Gateway and related client application libraries.

Upgrading to Fabric v2.5
^^^^^^^^^^^^^^^^^^^^^^^^

Hyperledger Fabric v2.5 will be maintained for an extended period of time.
Users of prior versions are encouraged to upgrade to Fabric v2.5 to continue receiving fixes and security updates in subsequent v2.5.x releases.
A simple in-place upgrade from the prior Fabric v2.2 LTS release is possible.
If you would like to upgrade from earlier releases there are additional considerations.
Please see the `Fabric v2.5 upgrade documentation <https://hyperledger-fabric.readthedocs.io/en/release-2.5/upgrade.html>`_ for complete details.

Release notes
=============

The release notes provide more details about each release.
Additionally, take a look at the announcements about changes and deprecations that are copied into each of the latest release notes.

* `Fabric v2.5.0 release notes <https://github.com/hyperledger/fabric/releases/tag/v2.5.0>`_.
* `Fabric v2.5.1 release notes <https://github.com/hyperledger/fabric/releases/tag/v2.5.1>`_.
* `Fabric v2.5.2 release notes <https://github.com/hyperledger/fabric/releases/tag/v2.5.2>`_.
* `Fabric v2.5.3 release notes <https://github.com/hyperledger/fabric/releases/tag/v2.5.3>`_.
* `Fabric v2.5.4 release notes <https://github.com/hyperledger/fabric/releases/tag/v2.5.4>`_.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
