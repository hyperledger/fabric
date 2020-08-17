Private Data
============

.. note:: This topic assumes an understanding of the conceptual material in the
          `documentation on private data <private-data/private-data.html>`_.

Private data collection definition
----------------------------------

A collection definition contains one or more collections, each having a policy
definition listing the organizations in the collection, as well as properties
used to control dissemination of private data at endorsement time and,
optionally, whether the data will be purged.

Beginning with the Fabric chaincode lifecycle introduced with Fabric v2.0, the
collection definition is part of the chaincode definition. The collection is
approved by channel members, and then deployed when the chaincode definition
is committed to the channel. The collection file needs to be the same for all
channel members. If you are using the peer CLI to approve and commit the
chaincode definition, use the ``--collections-config`` flag to specify the path
to the collection definition file. If you are using the Fabric SDK for Node.js,
visit `How to install and start your chaincode <https://hyperledger.github.io/fabric-sdk-node/{BRANCH}/tutorial-chaincode-lifecycle.html>`_.
To use the `previous lifecycle process <https://hyperledger-fabric.readthedocs.io/en/release-1.4/chaincode4noah.html>`_ to deploy a private data collection,
use the ``--collections-config`` flag when `instantiating your chaincode <https://hyperledger-fabric.readthedocs.io/en/{BRANCH_DOC}/commands/peerchaincode.html#peer-chaincode-instantiate>`_.

Collection definitions are composed of the following properties:

* ``name``: Name of the collection.

* ``policy``: The private data collection distribution policy defines which
  organizations' peers are allowed to retrieve and persist the collection data expressed using
  the ``Signature`` policy syntax, with each member being included in an ``OR``
  signature policy list. To support read/write transactions, the private data
  distribution policy must define a broader set of organizations than the
  endorsement policy, as peers must have the private data in order to endorse
  proposed transactions. For example, in a channel with ten organizations,
  five of the organizations might be included in a private data collection
  distribution policy, but the endorsement policy might call for any three
  of the organizations in the channel to endorse a read/write transaction.
  For write-only transactions, organizations that are not members of the
  collection distribution policy but are included in the chaincode level
  endorsement policy may endorse transactions that write to the private data
  collection. If this is not desirable, utilize a collection level
  ``endorsementPolicy`` to restrict the set of allowed endorsers to the private data
  distribution policy members.

* ``requiredPeerCount``: Minimum number of peers (across authorized organizations)
  that each endorsing peer must successfully disseminate private data to before the
  peer signs the endorsement and returns the proposal response back to the client.
  Requiring dissemination as a condition of endorsement will ensure that private data
  is available in the network even if the endorsing peer(s) become unavailable. When
  ``requiredPeerCount`` is ``0``, it means that no distribution is **required**,
  but there may be some distribution if ``maxPeerCount`` is greater than zero. A
  ``requiredPeerCount`` of ``0`` would typically not be recommended, as it could
  lead to loss of private data in the network if the endorsing peer(s) becomes unavailable.
  Typically you would want to require at least some distribution of the private
  data at endorsement time to ensure redundancy of the private data on multiple
  peers in the network.

* ``maxPeerCount``: For data redundancy purposes, the maximum number of other
  peers (across authorized organizations) that each endorsing peer will attempt
  to distribute the private data to. If an endorsing peer becomes unavailable between
  endorsement time and commit time, other peers that are collection members but who
  did not yet receive the private data at endorsement time, will be able to pull
  the private data from peers the private data was disseminated to. If this value
  is set to ``0``, the private data is not disseminated at endorsement time,
  forcing private data pulls against endorsing peers on all authorized peers at
  commit time.

* ``blockToLive``: Represents how long the data should live on the private
  database in terms of blocks. The data will live for this specified number of
  blocks on the private database and after that it will get purged, making this
  data obsolete from the network so that it cannot be queried from chaincode,
  and cannot be made available to requesting peers. To keep private data
  indefinitely, that is, to never purge private data, set the ``blockToLive``
  property to ``0``.

* ``memberOnlyRead``: a value of ``true`` indicates that peers automatically
  enforce that only clients belonging to one of the collection member organizations
  are allowed read access to private data. If a client from a non-member org
  attempts to execute a chaincode function that performs a read of a private data key,
  the chaincode invocation is terminated with an error. Utilize a value of
  ``false`` if you would like to encode more granular access control within
  individual chaincode functions.

* ``memberOnlyWrite``: a value of ``true`` indicates that peers automatically
  enforce that only clients belonging to one of the collection member organizations
  are allowed to write private data from chaincode. If a client from a non-member org
  attempts to execute a chaincode function that performs a write on a private data key,
  the chaincode invocation is terminated with an error. Utilize a value of
  ``false`` if you would like to encode more granular access control within
  individual chaincode functions, for example you may want certain clients
  from non-member organization to be able to create private data in a certain
  collection.

* ``endorsementPolicy``: An optional endorsement policy to utilize for the
  collection that overrides the chaincode level endorsement policy. A
  collection level endorsement policy may be specified in the form of a
  ``signaturePolicy`` or may be a ``channelConfigPolicy`` reference to
  an existing policy from the channel configuration. The ``endorsementPolicy``
  may be the same as the collection distribution ``policy``, or may require
  fewer or additional organization peers.

Here is a sample collection definition JSON file, containing an array of two
collection definitions:

.. code:: bash

 [
  {
     "name": "collectionMarbles",
     "policy": "OR('Org1MSP.member', 'Org2MSP.member')",
     "requiredPeerCount": 0,
     "maxPeerCount": 3,
     "blockToLive":1000000,
     "memberOnlyRead": true,
     "memberOnlyWrite": true
  },
  {
     "name": "collectionMarblePrivateDetails",
     "policy": "OR('Org1MSP.member')",
     "requiredPeerCount": 0,
     "maxPeerCount": 3,
     "blockToLive":3,
     "memberOnlyRead": true,
     "memberOnlyWrite":true,
     "endorsementPolicy": {
       "signaturePolicy": "OR('Org1MSP.member')"
     }
  }
 ]

This example uses the organizations from the Fabric test network, ``Org1`` and
``Org2``. The policy in the  ``collectionMarbles`` definition authorizes both
organizations to the private data. This is a typical configuration when the
chaincode data needs to remain private from the ordering service nodes. However,
the policy in the ``collectionMarblePrivateDetails`` definition restricts access
to a subset of organizations in the channel (in this case ``Org1`` ). Additionally,
writing to this collection requires endorsement from an ``Org1`` peer, even
though the chaincode level endorsement policy may require endorsement from
``Org1`` or ``Org2``. And since "memberOnlyWrite" is true, only clients from
``Org1`` may invoke chaincode that writes to the private data collection.
In this way you can control which organizations are entrusted to write to certain
private data collections.

Private data dissemination
--------------------------

Since private data is not included in the transactions that get submitted to
the ordering service, and therefore not included in the blocks that get distributed
to all peers in a channel, the endorsing peer plays an important role in
disseminating private data to other peers of authorized organizations. This ensures
the availability of private data in the channel's collection, even if endorsing
peers become unavailable after their endorsement. To assist with this dissemination,
the  ``maxPeerCount`` and ``requiredPeerCount`` properties in the collection definition
control the degree of dissemination at endorsement time.

If the endorsing peer cannot successfully disseminate the private data to at least
the ``requiredPeerCount``, it will return an error back to the client. The endorsing
peer will attempt to disseminate the private data to peers of different organizations,
in an effort to ensure that each authorized organization has a copy of the private
data. Since transactions are not committed at chaincode execution time, the endorsing
peer and recipient peers store a copy of the private data in a local ``transient store``
alongside their blockchain until the transaction is committed.

When authorized peers do not have a copy of the private data in their transient
data store at commit time (either because they were not an endorsing peer or because
they did not receive the private data via dissemination at endorsement time),
they will attempt to pull the private data from another authorized
peer, *for a configurable amount of time* based on the peer property
``peer.gossip.pvtData.pullRetryThreshold`` in the peer configuration ``core.yaml``
file.

.. note:: The peers being asked for private data will only return the private data
          if the requesting peer is a member of the collection as defined by the
          private data dissemination policy.

Considerations when using ``pullRetryThreshold``:

* If the requesting peer is able to retrieve the private data within the
  ``pullRetryThreshold``, it will commit the transaction to its ledger
  (including the private data hash), and store the private data in its
  state database, logically separated from other channel state data.

* If the requesting peer is not able to retrieve the private data within
  the ``pullRetryThreshold``, it will commit the transaction to it’s blockchain
  (including the private data hash), without the private data.

* If the peer was entitled to the private data but it is missing, then
  that peer will not be able to endorse future transactions that reference
  the missing private data - a chaincode query for a key that is missing will
  be detected (based on the presence of the key’s hash in the state database),
  and the chaincode will receive an error.

Therefore, it is important to set the ``requiredPeerCount`` and ``maxPeerCount``
properties large enough to ensure the availability of private data in your
channel. For example, if each of the endorsing peers become unavailable
before the transaction commits, the ``requiredPeerCount`` and ``maxPeerCount``
properties will have ensured the private data is available on other peers.

.. note:: For collections to work, it is important to have cross organizational
          gossip configured correctly. Refer to our documentation on :doc:`gossip`,
          paying particular attention to the "anchor peers" and "external endpoint"
          configuration.

Referencing collections from chaincode
--------------------------------------

A set of `shim APIs <https://godoc.org/github.com/hyperledger/fabric-chaincode-go/shim>`_
are available for setting and retrieving private data.

The same chaincode data operations can be applied to channel state data and
private data, but in the case of private data, a collection name is specified
along with the data in the chaincode APIs, for example
``PutPrivateData(collection,key,value)`` and ``GetPrivateData(collection,key)``.

A single chaincode can reference multiple collections.

Referencing implicit collections from chaincode
-----------------------------------------------

Starting in v2.0, an implicit private data collection can be used for each
organization in a channel, so that you don't have to define collections if you'd
like to utilize per-organization collections. Each org-specific implicit collection
has a distribution policy and endorsement policy of the matching organization.
You can therefore utilize implicit collections for use cases where you'd like
to ensure that a specific organization has written to a collection key namespace.
The v2.0 chaincode lifecycle uses implicit collections to track which organizations
have approved a chaincode definition. Similarly, you can use implicit collections
in application chaincode to track which organizations have approved or voted
for some change in state.

To write and read an implicit private data collection key, in the ``PutPrivateData``
and ``GetPrivateData`` chaincode APIs, specify the collection parameter as
``"_implicit_org_<MSPID>"``, for example ``"_implicit_org_Org1MSP"``.

.. note:: Application defined collection names are not allowed to start with an underscore,
          therefore there is no chance for an implicit collection name to collide
          with an application defined collection name.

How to pass private data in a chaincode proposal
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Since the chaincode proposal gets stored on the blockchain, it is also important
not to include private data in the main part of the chaincode proposal. A special
field in the chaincode proposal called the ``transient`` field can be used to pass
private data from the client (or data that chaincode will use to generate private
data), to chaincode invocation on the peer.  The chaincode can retrieve the
``transient`` field by calling the `GetTransient() API <https://godoc.org/github.com/hyperledger/fabric-chaincode-go/shim#ChaincodeStub.GetTransient>`_.
This ``transient`` field gets excluded from the channel transaction.

Protecting private data content
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
If the private data is relatively simple and predictable (e.g. transaction dollar
amount), channel members who are not authorized to the private data collection
could try to guess the content of the private data via brute force hashing of
the domain space, in hopes of finding a match with the private data hash on the
chain. Private data that is predictable should therefore include a random "salt"
that is concatenated with the private data key and included in the private data
value, so that a matching hash cannot realistically be found via brute force.
The random "salt" can be generated at the client side (e.g. by sampling a secure
pseudo-random source) and then passed along with the private data in the transient
field at the time of chaincode invocation.

Access control for private data
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Until version 1.3, access control to private data based on collection membership
was enforced for peers only. Access control based on the organization of the
chaincode proposal submitter was required to be encoded in chaincode logic.
Collection configuration options ``memberOnlyRead`` (since version v1.4) and
``memberOnlyWrite`` (since version v2.0) can automatically enforce that the chaincode
proposal submitter must be from a collection member in order to read or write
private data keys. For more information about collection
configuration definitions and how to set them, refer back to the
`Private data collection definition`_  section of this topic.

.. note:: If you would like more granular access control, you can set
          ``memberOnlyRead`` and ``memberOnlyWrite`` to false. You can then apply your
          own access control logic in chaincode, for example by calling the GetCreator()
          chaincode API or using the client identity
          `chaincode library <https://godoc.org/github.com/hyperledger/fabric-chaincode-go/shim#ChaincodeStub.GetCreator>`__ .

Querying Private Data
~~~~~~~~~~~~~~~~~~~~~

Private data collection can be queried just like normal channel data, using
shim APIs:

* ``GetPrivateDataByRange(collection, startKey, endKey string)``
* ``GetPrivateDataByPartialCompositeKey(collection, objectType string, keys []string)``

And for the CouchDB state database, JSON content queries can be passed using the
shim API:

* ``GetPrivateDataQueryResult(collection, query string)``

Limitations:

* Clients that call chaincode that executes range or rich JSON queries should be aware
  that they may receive a subset of the result set, if the peer they query has missing
  private data, based on the explanation in Private Data Dissemination section
  above.  Clients can query multiple peers and compare the results to
  determine if a peer may be missing some of the result set.
* Chaincode that executes range or rich JSON queries and updates data in a single
  transaction is not supported, as the query results cannot be validated on the peers
  that don’t have access to the private data, or on peers that are missing the
  private data that they have access to. If a chaincode invocation both queries
  and updates private data, the proposal request will return an error. If your application
  can tolerate result set changes between chaincode execution and validation/commit time,
  then you could call one chaincode function to perform the query, and then call a second
  chaincode function to make the updates. Note that calls to GetPrivateData() to retrieve
  individual keys can be made in the same transaction as PutPrivateData() calls, since
  all peers can validate key reads based on the hashed key version.

Using Indexes with collections
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The topic :doc:`couchdb_as_state_database` describes indexes that can be
applied to the channel’s state database to enable JSON content queries, by
packaging indexes in a ``META-INF/statedb/couchdb/indexes`` directory at chaincode
installation time.  Similarly, indexes can also be applied to private data
collections, by packaging indexes in a ``META-INF/statedb/couchdb/collections/<collection_name>/indexes``
directory. An example index is available `here <https://github.com/hyperledger/fabric-samples/blob/{BRANCH}/chaincode/marbles02_private/go/META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexOwner.json>`_.

Considerations when using private data
--------------------------------------

Private data purging
~~~~~~~~~~~~~~~~~~~~

Private data can be periodically purged from peers. For more details,
see the ``blockToLive`` collection definition property above.

Additionally, recall that prior to commit, peers store private data in a local
transient data store. This data automatically gets purged when the transaction
commits.  But if a transaction was never submitted to the channel and
therefore never committed, the private data would remain in each peer’s
transient store.  This data is purged from the transient store after a
configurable number blocks by using the peer’s
``peer.gossip.pvtData.transientstoreMaxBlockRetention`` property in the peer
``core.yaml`` file.

Updating a collection definition
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

To update a collection definition or add a new collection, you can update
the chaincode definition and pass the new collection configuration
in the chaincode approve and commit transactions, for example using the ``--collections-config``
flag if using the CLI. If a collection configuration is specified when updating
the chaincode definition, a definition for each of the existing collections must be
included.

When updating a chaincode definition, you can add new private data collections,
and update existing private data collections, for example to add new
members to an existing collection or change one of the collection definition
properties. Note that you cannot update the collection name or the
blockToLive property, since a consistent blockToLive is required
regardless of a peer's block height.

Collection updates becomes effective when a peer commits the block with the updated
chaincode definition. Note that collections cannot be
deleted, as there may be prior private data hashes on the channel’s blockchain
that cannot be removed.

Private data reconciliation
~~~~~~~~~~~~~~~~~~~~~~~~~~~

Starting in v1.4, peers of organizations that are added to an existing collection
will automatically fetch private data that was committed to the collection before
they joined the collection.

This private data "reconciliation" also applies to peers that
were entitled to receive private data but did not yet receive it --- because of
a network failure, for example --- by keeping track of private data that was "missing"
at the time of block commit.

Private data reconciliation occurs periodically based on the
``peer.gossip.pvtData.reconciliationEnabled`` and ``peer.gossip.pvtData.reconcileSleepInterval``
properties in core.yaml. The peer will periodically attempt to fetch the private
data from other collection member peers that are expected to have it.

Note that this private data reconciliation feature only works on peers running
v1.4 or later of Fabric.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
