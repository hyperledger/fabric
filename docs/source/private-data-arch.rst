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

The collection definition gets deployed to the channel at the time of chaincode
instantiation (or upgrade). If using the peer CLI to instantiate the chaincode, the
collection definition file is passed to the chaincode instantiation
using the ``--collections-config`` flag. If using a client SDK, check the `SDK
documentation <https://fabric-sdk-node.github.io/>`_ for information on providing the collection
definition.

Collection definitions are composed of five properties:

* ``name``: Name of the collection.

* ``policy``: The private data collection distribution policy defines which
  organizations' peers are allowed to persist the collection data expressed using
  the ``Signature`` policy syntax, with each member being included in an ``OR``
  signature policy list. To support read/write transactions, the private data
  distribution policy must define a broader set of organizations than the chaincode
  endorsement policy, as peers must have the private data in order to endorse
  proposed transactions. For example, in a channel with ten organizations,
  five of the organizations might be included in a private data collection
  distribution policy, but the endorsement policy might call for any three
  of the organizations to endorse.

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
  data obsolete from the network. To keep private data indefinitely, that is, to
  never purge private data, set the ``blockToLive`` property to ``0``.

Here is a sample collection definition JSON file, containing an array of two
collection definitions:

.. code:: bash

 [
  {
     "name": "collectionMarbles",
     "policy": "OR('Org1MSP.member', 'Org2MSP.member')",
     "requiredPeerCount": 0,
     "maxPeerCount": 3,
     "blockToLive":1000000
  },
  {
     "name": "collectionMarblePrivateDetails",
     "policy": "OR('Org1MSP.member')",
     "requiredPeerCount": 0,
     "maxPeerCount": 3,
     "blockToLive":3
  }
 ]

This example uses the organizations from the BYFN sample network, ``Org1`` and
``Org2`` . The policy in the  ``collectionMarbles`` definition authorizes both
organizations to the private data. This is a typical configuration when the
chaincode data needs to remain private from the ordering service nodes. However,
the policy in the ``collectionMarblePrivateDetails`` definition restricts access
to a subset of organizations in the channel (in this case ``Org1`` ). In a real
scenario, there would be many organizations in the channel, with two or more
organizations in each collection sharing private data between them.

Endorsement
~~~~~~~~~~~

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

How private data is committed
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

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
          paying particular attention to the section on "anchor peers".

Referencing collections from chaincode
--------------------------------------

A set of `shim APIs <https://godoc.org/github.com/hyperledger/fabric/core/chaincode/shim>`_
are available for setting and retrieving private data.

The same chaincode data operations can be applied to channel state data and
private data, but in the case of private data, a collection name is specified
along with the data in the chaincode APIs, for example
``PutPrivateData(collection,key,value)`` and ``GetPrivateData(collection,key)``.

A single chaincode can reference multiple collections.

How to pass private data in a chaincode proposal
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Since the chaincode proposal gets stored on the blockchain, it is also important
not to include private data in the main part of the chaincode proposal. A special
field in the chaincode proposal called the ``transient`` field can be used to pass
private data from the client (or data that chaincode will use to generate private
data), to chaincode invocation on the peer.  The chaincode can retrieve the
``transient`` field by calling the `GetTransient() API <https://github.com/hyperledger/fabric/blob/8b3cbda97e58d1a4ff664219244ffd1d89d7fba8/core/chaincode/shim/interfaces.go#L315-L321>`_.
This ``transient`` field gets excluded from the channel transaction.

Considerations when using private data
--------------------------------------

Querying Private Data
~~~~~~~~~~~~~~~~~~~~~

Private collection data can be queried just like normal channel data, using
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
* Note that private data collections only define which organization’s peers
  are authorized to receive and store private data, and consequently implies
  which peers can be used to query private data. Private data collections do not
  by themselves limit access control within chaincode. For example if
  non-authorized clients are able to invoke chaincode on peers that have access
  to the private data, the chaincode logic still needs a means to enforce access
  control as usual, for example by calling the GetCreator() chaincode API or
  using the client identity `chaincode library <https://github.com/hyperledger/fabric/tree/master/core/chaincode/lib/cid>`__ .

Using Indexes with collections
------------------------------

The topic :doc:`couchdb_as_state_database` describes indexes that can be
applied to the channel’s state database to enable JSON content queries, by
packaging indexes in a ``META-INF/statedb/couchdb/indexes`` directory at chaincode
installation time.  Similarly, indexes can also be applied to private data
collections, by packaging indexes in a ``META-INF/statedb/couchdb/collections/<collection_name>/indexes``
directory. An example index is available `here <https://github.com/hyperledger/fabric-samples/blob/master/chaincode/marbles02_private/go/META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexOwner.json>`_.

Private Data Purging
~~~~~~~~~~~~~~~~~~~~

To keep private data indefinitely, that is, to never purge private data,
set ``blockToLive`` property to ``0``.

Recall that prior to commit, peers store private data in a local
transient data store. This data automatically gets purged when the transaction
commits.  But if a transaction was never submitted to the channel and
therefore never committed, the private data would remain in each peer’s
transient store.  This data is purged from the transient store after a
configurable number blocks by using the peer’s
``peer.gossip.pvtData.transientstoreMaxBlockRetention`` property in the peer
``core.yaml`` file.

Upgrading a collection definition
---------------------------------

If a collection is referenced by a chaincode, the chaincode will use the prior
collection definition unless a new collection definition is specified at upgrade
time. If a collection configuration is specified during the upgrade, a definition
for each of the existing collections must be included, and you can add new
collection definitions.

Collection updates becomes effective when a peer commits the block that
contains the chaincode upgrade transaction. Note that collections cannot be
deleted, as there may be prior private data hashes on the channel’s blockchain
that cannot be removed.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
