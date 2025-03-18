What's new in Hyperledger Fabric
================================

What's New in Hyperledger Fabric v3.1
-------------------------------------

Performance optimization - Batching of chaincode writes
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Chaincodes that write large numbers of keys were inefficient since each key write required communication between the chaincode and the peer.
The new feature enables a chaincode developer to batch multiple writes into a single communication between the chaincode and the peer.
A batch can be started by calling ``StartWriteBatch()``. The chaincode can then perform key writes as usual.
Then when ``FinishWriteBatch()`` is called or the transaction execution ends the writes will be sent to the peer.
Batches over a configured size will be split into multiple batch segments.
The batching approach significantly improves performance for chaincodes that write to many keys.
Note that the batching only impacts the endorsement phase. The transaction itself, as well as the validation and commit phases, remains the same as previous versions.

The batch writes are configured with the following peer core.yaml chaincode properties:
 * ``chaincode.runtimeParams.useWriteBatch`` - boolean that indicates whether write batching is enabled on peer
 * ``chaincode.runtimeParams.maxSizeWriteBatch`` - integer that indicates the maximum number of keys written per batch segment.

The new feature requires chaincode to utilize fabric-chaincode-go v2.1.0 or higher.

Performance optimization - Batching of chaincode reads
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Chaincodes that read large numbers of keys were inefficient since each key read required communication between the chaincode and the peer.
The new feature enables a chaincode developer to batch multiple reads into a single communication between the chaincode and the peer.
Utilize the new ``GetMultipleStates()`` and ``GetMultiplePrivateData()`` chaincode functions to perform the batch reads.
Batches over a configured size will be split into multiple batch segments.

The batch reads are configured with the following peer core.yaml chaincode properties:
 * ``chaincode.runtimeParams.useGetMultipleKeys`` - boolean that indicates whether read batching is enabled on peer
 * ``chaincode.runtimeParams.maxSizeGetMultipleKeys`` - integer that indicates the maximum number of keys read per batch segment

The new feature requires chaincode to utilize fabric-chaincode-go v2.2.0 or higher.

Query all composite keys in a chaincode
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

A new chaincode function ``GetAllStatesCompositeKeyWithPagination()`` is available so that all composite keys within a chaincode can be retrieved.
This function is useful when performing bulk operations on all composite keys in a chaincode.

The new feature requires chaincode to utilize fabric-chaincode-go v2.3.0 or higher.


What's New in Hyperledger Fabric v3.0
-------------------------------------

Byzantine Fault Tolerant (BFT) ordering service
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Hyperledger Fabric has utilized a Raft crash fault tolerant (CFT) ordering service since version v1.4.
A Byzantine Fault Tolerant (BFT) ordering service can withstand not only crash failures, but also a subset of nodes behaving maliciously.
Fabric v3.0 is the first release to provide a BFT ordering service based on the
`SmartBFT <https://arxiv.org/abs/2107.06922>`_ `consensus library <https://github.com/hyperledger-labs/SmartBFT>`_.
Consider using the BFT orderer if true decentralization is required,
where up to and not including a third of the parties running the orderers may not be trusted due to malicious intent or being compromised.

Channel capability `V3_0` must be enabled to utilize SmartBFT consensus.

Learn more about the BFT ordering service in the **BFT** section of the :doc:`orderer/ordering_service` documentation.

For more details see the :doc:`bft_configuration` topic.

Try it out for yourself! Look for the **Bring up the network using BFT ordering service** in the :doc:`test_network` tutorial.
Alternatively, you can run the binaries locally by following the steps in the `test-network-nano-bash sample <https://github.com/hyperledger/fabric-samples/tree/main/test-network-nano-bash>`_.

Support for Ed25519
^^^^^^^^^^^^^^^^^^^

Ed25519 cryptographic algorithm is now supported in addition to ECDSA for MSP functions including transaction signing and verification.

Channel capability `V3_0` must be enabled to utilize certificates with Ed25519 keys.

Hyperledger Fabric v2.5
-----------------------

Hyperledger Fabric v2.5 remains the long-term support (LTS) release for production users.
Check out the `What's New in v2.5 topic <https://hyperledger-fabric.readthedocs.io/en/release-2.5/whatsnew.html>`_
to learn about features since the prior v2.2 LTS release, including
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

* `Fabric v3.0.0 release notes <https://github.com/hyperledger/fabric/releases/tag/v3.0.0>`_.
* `Fabric v3.1.0 release notes <https://github.com/hyperledger/fabric/releases/tag/v3.1.0>`_.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
