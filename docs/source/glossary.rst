Glossary
===========================

Terminology is important, so that all Hyperledger Fabric users and developers
agree on what we mean by each specific term. What is a smart contract for
example. The documentation will reference the glossary as needed, but feel free
to read the entire thing in one sitting if you like; it's pretty enlightening!

.. _Anchor-Peer:

Anchor Peer
-----------

Used by gossip to make sure peers in different organizations know about each other.

When a configuration block that contains an update to the anchor peers is committed,
peers reach out to the anchor peers and learn from them about all of the peers known
to the anchor peer(s). Once at least one peer from each organization has contacted an
anchor peer, the anchor peer learns about every peer in the channel. Since gossip
communication is constant, and because peers always ask to be told about the existence
of any peer they don't know about, a common view of membership can be established for
a channel.

For example, let's assume we have three organizations --- ``A``, ``B``, ``C`` --- in the channel
and a single anchor peer --- ``peer0.orgC`` --- defined for organization ``C``.
When ``peer1.orgA`` (from organization ``A``) contacts ``peer0.orgC``, it will
tell ``peer0.orgC`` about ``peer0.orgA``. And when at a later time ``peer1.orgB``
contacts ``peer0.orgC``, the latter would tell the former about ``peer0.orgA``.
From that point forward, organizations ``A`` and ``B`` would start exchanging
membership information directly without any assistance from ``peer0.orgC``.

As communication across organizations depends on gossip in order to work, there must
be at least one anchor peer defined in the channel configuration. It is strongly
recommended that every organization provides its own set of anchor peers for high
availability and redundancy.

.. _glossary_ACL:

ACL
---

An ACL, or Access Control List, associates access to specific peer
resources (such as system chaincode APIs or event services) to a Policy_
(which specifies how many and what types of organizations or roles are
required). The ACL is part of a channel's configuration. It is therefore
persisted in the channel's configuration blocks, and can be updated using the
standard configuration update mechanism.

An ACL is formatted as a list of key-value pairs, where the key identifies
the resource whose access we wish to control, and the value identifies the
channel policy (group) that is allowed to access it. For example
``lscc/GetDeploymentSpec: /Channel/Application/Readers``
defines that the access to the life cycle chaincode ``GetDeploymentSpec`` API
(the resource) is accessible by identities which satisfy the
``/Channel/Application/Readers`` policy.

A set of default ACLs is provided in the ``configtx.yaml`` file which is
used by configtxgen to build channel configurations. The defaults can be set
in the top level "Application" section of ``configtx.yaml`` or overridden
on a per profile basis in the "Profiles" section.

.. _Block:

Block
-----

.. figure:: ./glossary/glossary.block.png
   :scale: 50 %
   :align: right
   :figwidth: 40 %
   :alt: A Block

   Block B1 is linked to block B0. Block B2 is linked to block B1.

=======

A block contains an ordered set of transactions. It is cryptographically linked
to the preceding block, and in turn it is linked to be subsequent blocks. The
first block in such a chain of blocks is called the **genesis block**. Blocks
are created by the ordering system, and validated by peers.


.. _Chain:


Chain
-----

.. figure:: ./glossary/glossary.blockchain.png
   :scale: 75 %
   :align: right
   :figwidth: 40 %
   :alt: Blockchain

   Blockchain B contains blocks 0, 1, 2.

=======

The ledger's chain is a transaction log structured as hash-linked blocks of
transactions. Peers receive blocks of transactions from the ordering service, mark
the block's transactions as valid or invalid based on endorsement policies and
concurrency violations, and append the block to the hash chain on the peer's
file system.

.. _chaincode:

Chaincode
---------

See Smart-Contract_.

.. _Channel:


Channel
-------

.. figure:: ./glossary/glossary.channel.png
   :scale: 30 %
   :align: right
   :figwidth: 40 %
   :alt: A Channel

   Channel C connects application A1, peer P2 and ordering service O1.

=======

A channel is a private blockchain overlay which allows for data
isolation and confidentiality. A channel-specific ledger is shared across the
peers in the channel, and transacting parties must be properly authenticated to
a channel in order to interact with it.  Channels are defined by a
Configuration-Block_.


.. _Commit:

Commit
------

Each Peer_ on a channel validates ordered blocks of
transactions and then commits (writes/appends) the blocks to its replica of the
channel Ledger_. Peers also mark each transaction in each block
as valid or invalid.

.. _Concurrency-Control-Version-Check:

Concurrency Control Version Check
---------------------------------

Concurrency Control Version Check is a method of keeping state in sync across
peers on a channel. Peers execute transactions in parallel, and before commitment
to the ledger, peers check that the data read at execution time has not changed.
If the data read for the transaction has changed between execution time and
commitment time, then a Concurrency Control Version Check violation has
occurred, and the transaction is marked as invalid on the ledger and values
are not updated in the state database.

.. _Configuration-Block:

Configuration Block
-------------------

Contains the configuration data defining members and policies for a system
chain (ordering service) or channel. Any configuration modifications to a
channel or overall network (e.g. a member leaving or joining) will result
in a new configuration block being appended to the appropriate chain. This
block will contain the contents of the genesis block, plus the delta.

.. _Consensus:

Consensus
---------

A broader term overarching the entire transactional flow, which serves to generate
an agreement on the order and to confirm the correctness of the set of transactions
constituting a block.

.. _Consortium:

Consenter set
-------------

In a Raft ordering service, these are the ordering nodes actively participating
in the consensus mechanism on a channel. If other ordering nodes exist on the
system channel, but are not a part of a channel, they are not part of that
channel's consenter set.

.. _Consenter-Set:

Consortium
----------

A consortium is a collection of non-orderer organizations on the blockchain
network. These are the organizations that form and join channels and that own
peers. While a blockchain network can have multiple consortia, most blockchain
networks have a single consortium. At channel creation time, all organizations
added to the channel must be part of a consortium. However, an organization
that is not defined in a consortium may be added to an existing channel.

.. _Consortium:

Current State
-------------

See World-State_.

.. _Current-State:

.. _Dynamic-Membership:

Dynamic Membership
------------------

Hyperledger Fabric supports the addition/removal of members, peers, and ordering service
nodes, without compromising the operationality of the overall network. Dynamic
membership is critical when business relationships adjust and entities need to
be added/removed for various reasons.

.. _Endorsement:

Endorsement
-----------

Refers to the process where specific peer nodes execute a chaincode transaction and return
a proposal response to the client application. The proposal response includes the
chaincode execution response message, results (read set and write set), and events,
as well as a signature to serve as proof of the peer's chaincode execution.
Chaincode applications have corresponding endorsement policies, in which the endorsing
peers are specified.

.. _Endorsement-policy:

Endorsement policy
------------------

Defines the peer nodes on a channel that must execute transactions attached to a
specific chaincode application, and the required combination of responses (endorsements).
A policy could require that a transaction be endorsed by a minimum number of
endorsing peers, a minimum percentage of endorsing peers, or by all endorsing
peers that are assigned to a specific chaincode application. Policies can be
curated based on the application and the desired level of resilience against
misbehavior (deliberate or not) by the endorsing peers. A transaction that is submitted
must satisfy the endorsement policy before being marked as valid by committing peers.
A distinct endorsement policy for install and instantiate transactions is also required.

.. _Follower:

Follower
--------

In a leader based consensus protocol, such as Raft, these are the nodes which
replicate log entries produced by the leader. In Raft, the followers also receive
"heartbeat" messages from the leader. In the event that the leader stops sending
those message for a configurable amount of time, the followers will initiate a
leader election and one of them will be elected leader.

.. _Genesis-Block:

Genesis Block
-------------

The configuration block that initializes the ordering service, or serves as the
first block on a chain.

.. _Gossip-Protocol:

Gossip Protocol
---------------

The gossip data dissemination protocol performs three functions:
1) manages peer discovery and channel membership;
2) disseminates ledger data across all peers on the channel;
3) syncs ledger state across all peers on the channel.
Refer to the :doc:`Gossip <gossip>` topic for more details.

.. _Fabric-ca:

Hyperledger Fabric CA
---------------------

Hyperledger Fabric CA is the default Certificate Authority component, which
issues PKI-based certificates to network member organizations and their users.
The CA issues one root certificate (rootCert) to each member and one enrollment
certificate (ECert) to each authorized user.

.. _Initialize:

Initialize
----------

A method to initialize a chaincode application.

Install
-------

The process of placing a chaincode on a peer's file system.

Instantiate
-----------

The process of starting and initializing a chaincode application on a specific channel.
After instantiation, peers that have the chaincode installed can accept chaincode
invocations.

.. _Invoke:

Invoke
------

Used to call chaincode functions. A client application invokes chaincode by
sending a transaction proposal to a peer. The peer will execute the chaincode
and return an endorsed proposal response to the client application. The client
application will gather enough proposal responses to satisfy an endorsement policy,
and will then submit the transaction results for ordering, validation, and commit.
The client application may choose not to submit the transaction results. For example
if the invoke only queried the ledger, the client application typically would not
submit the read-only transaction, unless there is desire to log the read on the ledger
for audit purpose. The invoke includes a channel identifier, the chaincode function to
invoke, and an array of arguments.

.. _Leader

Leader
------

In a leader based consensus protocol, like Raft, the leader is responsible for
ingesting new log entries, replicating them to follower ordering nodes, and
managing when an entry is considered committed. This is not a special **type**
of orderer. It is only a role that an orderer may have at certain times, and
then not others, as circumstances determine.

.. _Leading-Peer:

Leading Peer
------------

Each Organization_ can own multiple peers on each channel that
they subscribe to. One or more of these peers should serve as the leading peer
for the channel, in order to communicate with the network ordering service on
behalf of the organization. The ordering service delivers blocks to the
leading peer(s) on a channel, who then distribute them to other peers within
the same organization.

.. _Ledger:

Ledger
------

.. figure:: ./glossary/glossary.ledger.png
   :scale: 25 %
   :align: right
   :figwidth: 20 %
   :alt: A Ledger

   A Ledger, 'L'


A ledger consists of two distinct, though related, parts -- a "blockchain" and
the "state database", also known as "world state". Unlike other ledgers,
blockchains are **immutable** -- that is, once a block has been added to the
chain, it cannot be changed. In contrast, the "world state" is a database
containing the current value of the set of key-value pairs that have been added,
modified or deleted by the set of validated and committed transactions in the
blockchain.

It's helpful to think of there being one **logical** ledger for each channel in
the network. In reality, each peer in a channel maintains its own copy of the
ledger -- which is kept consistent with every other peer's copy through a
process called **consensus**. The term **Distributed Ledger Technology**
(**DLT**) is often associated with this kind of ledger -- one that is logically
singular, but has many identical copies distributed across a set of network
nodes (peers and the ordering service).

.. _Log-entry

Log entry
---------

The primary unit of work in a Raft ordering service, log entries are distributed
from the leader orderer to the followers. The full sequence of such entries known
as the "log". The log is considered to be consistent if all members agree on the
entries and their order.

.. _Member:

Member
------

See Organization_.

.. _MSP:

Membership Service Provider
---------------------------

.. figure:: ./glossary/glossary.msp.png
   :scale: 35 %
   :align: right
   :figwidth: 25 %
   :alt: An MSP

   An MSP, 'ORG.MSP'


The Membership Service Provider (MSP) refers to an abstract component of the
system that provides credentials to clients, and peers for them to participate
in a Hyperledger Fabric network. Clients use these credentials to authenticate
their transactions, and peers use these credentials to authenticate transaction
processing results (endorsements). While strongly connected to the transaction
processing components of the systems, this interface aims to have membership
services components defined, in such a way that alternate implementations of
this can be smoothly plugged in without modifying the core of transaction
processing components of the system.

.. _Membership-Services:

Membership Services
-------------------

Membership Services authenticates, authorizes, and manages identities on a
permissioned blockchain network. The membership services code that runs in peers
and orderers both authenticates and authorizes blockchain operations.  It is a
PKI-based implementation of the Membership Services Provider (MSP) abstraction.

.. _Ordering-Service:

Ordering Service
----------------

A defined collective of nodes that orders transactions into a block.  The ordering
service exists independent of the peer processes and orders transactions on a
first-come-first-serve basis for all channel's on the network.  The ordering service is
designed to support pluggable implementations beyond the out-of-the-box SOLO and Kafka varieties.
The ordering service is a common binding for the overall network; it contains the cryptographic
identity material tied to each Member_.

.. _Organization:

Organization
------------

=====


.. figure:: ./glossary/glossary.organization.png
   :scale: 25 %
   :align: right
   :figwidth: 20 %
   :alt: An Organization

   An organization, 'ORG'


Also known as "members", organizations are invited to join the blockchain network
by a blockchain service provider. An organization is joined to a network by adding its
Membership Service Provider (MSP_) to the network. The MSP defines how other members of the
network may verify that signatures (such as those over transactions) were generated by a valid
identity, issued by that organization. The particular access rights of identities within an MSP
are governed by policies which are also agreed upon when the organization is joined to the
network. An organization can be as large as a multi-national corporation or as small as an
individual. The transaction endpoint of an organization is a Peer_. A collection of organizations
form a Consortium_. While all of the organizations on a network are members, not every organization
will be part of a consortium.

.. _Peer:

Peer
----

.. figure:: ./glossary/glossary.peer.png
   :scale: 25 %
   :align: right
   :figwidth: 20 %
   :alt: A Peer

   A peer, 'P'

A network entity that maintains a ledger and runs chaincode containers in order to perform
read/write operations to the ledger.  Peers are owned and maintained by members.

.. _Policy:

Policy
------

Policies are expressions composed of properties of digital identities, for
example: ``Org1.Peer OR Org2.Peer``. They are used to restrict access to
resources on a blockchain network. For instance, they dictate who can read from
or write to a channel, or who can use a specific chaincode API via an ACL_.
Policies may be defined in ``configtx.yaml`` prior to bootstrapping an ordering
service or creating a channel, or they can be specified when instantiating
chaincode on a channel. A default set of policies ship in the sample
``configtx.yaml`` which will be appropriate for most networks.

.. _glossary-Private-Data:

Private Data
------------

Confidential data that is stored in a private database on each authorized peer,
logically separate from the channel ledger data. Access to this data is
restricted to one or more organizations on a channel via a private data
collection definition. Unauthorized organizations will have a hash of the
private data on the channel ledger as evidence of the transaction data. Also,
for further privacy, hashes of the private data go through the
Ordering-Service_, not the private data itself, so this keeps private data
confidential from Orderer.

.. _glossary-Private-Data-Collection:

Private Data Collection (Collection)
------------------------------------

Used to manage confidential data that two or more organizations on a channel
want to keep private from other organizations on that channel. The collection
definition describes a subset of organizations on a channel entitled to store
a set of private data, which by extension implies that only these organizations
can transact with the private data.

.. _Proposal:

Proposal
--------

A request for endorsement that is aimed at specific peers on a channel. Each
proposal is either an instantiate or an invoke (read/write) request.

.. _Query:

Query
-----

A query is a chaincode invocation which reads the ledger current state but does
not write to the ledger. The chaincode function may query certain keys on the ledger,
or may query for a set of keys on the ledger. Since queries do not change ledger state,
the client application will typically not submit these read-only transactions for ordering,
validation, and commit. Although not typical, the client application can choose to
submit the read-only transaction for ordering, validation, and commit, for example if the
client wants auditable proof on the ledger chain that it had knowledge of specific ledger
state at a certain point in time.

.. _Quorum:

Quorum
------

This describes the minimum number of members of the cluster that need to
affirm a proposal so that transactions can be ordered. For every consenter set,
this is a **majority** of nodes. In a cluster with five nodes, three must be
available for there to be a quorum. If a quorum of nodes is unavailable for any
reason, the cluster becomes unavailable for both read and write operations and
no new logs can be committed.

.. _Raft:

Raft
----

New for v1.4.1, Raft is a crash fault tolerant (CFT) ordering service
implementation based on the `etcd library <https://coreos.com/etcd/>`_
of the `Raft protocol` <https://raft.github.io/raft.pdf>`_. Raft follows a
"leader and follower" model, where a leader node is elected (per channel) and
its decisions are replicated by the followers. Raft ordering services should
be easier to set up and manage than Kafka-based ordering services, and their
design allows organizations to contribute nodes to a distributed ordering
service.

.. _SDK:

Software Development Kit (SDK)
------------------------------

The Hyperledger Fabric client SDK provides a structured environment of libraries
for developers to write and test chaincode applications. The SDK is fully
configurable and extensible through a standard interface. Components, including
cryptographic algorithms for signatures, logging frameworks and state stores,
are easily swapped in and out of the SDK. The SDK provides APIs for transaction
processing, membership services, node traversal and event handling.

Currently, the two officially supported SDKs are for Node.js and Java, while three
more -- Python, Go and REST -- are not yet official but can still be downloaded
and tested.

.. _Smart-Contract:

Smart Contract
--------------

A smart contract is code -- invoked by a client application external to the
blockchain network -- that manages access and modifications to a set of
key-value pairs in the :ref:`World-State`. In Hyperledger Fabric, smart
contracts are referred to as chaincode. Smart contract chaincode is installed
onto peer nodes and instantiated to one or more channels.

.. _State-DB:

State Database
--------------

Current state data is stored in a state database for efficient reads and queries
from chaincode. Supported databases include levelDB and couchDB.

.. _System-Chain:

System Chain
------------

Contains a configuration block defining the network at a system level. The
system chain lives within the ordering service, and similar to a channel, has
an initial configuration containing information such as: MSP information, policies,
and configuration details.  Any change to the overall network (e.g. a new org
joining or a new ordering node being added) will result in a new configuration block
being added to the system chain.

The system chain can be thought of as the common binding for a channel or group
of channels.  For instance, a collection of financial institutions may form a
consortium (represented through the system chain), and then proceed to create
channels relative to their aligned and varying business agendas.

.. _Transaction:

Transaction
-----------

.. figure:: ./glossary/glossary.transaction.png
   :scale: 30 %
   :align: right
   :figwidth: 20 %
   :alt: A Transaction

   A transaction, 'T'

Invoke or instantiate results that are submitted for ordering, validation, and commit.
Invokes are requests to read/write data from the ledger. Instantiate is a request to
start and initialize a chaincode on a channel. Application clients gather invoke or
instantiate responses from endorsing peers and package the results and endorsements
into a transaction that is submitted for ordering, validation, and commit.

.. _World-State:

World State
-----------

.. figure:: ./glossary/glossary.worldstate.png
   :scale: 40 %
   :align: right
   :figwidth: 25 %
   :alt: Current State

   The World State, 'W'

Also known as the “current state”, the world state is a component of the
HyperLedger Fabric :ref:`Ledger`. The world state represents the latest values
for all keys included in the chain transaction log. Chaincode executes
transaction proposals against world state data because the world state provides
direct access to the latest value of these keys rather than having to calculate
them by traversing the entire transaction log. The world state will change
every time the value of a key changes (for example, when the ownership of a
car -- the "key" -- is transferred from one owner to another -- the
"value") or when a new key is added (a car is created). As a result, the world
state is critical to a transaction flow, since the current state of a key-value
pair must be known before it can be changed. Peers commit the latest values to
the ledger world state for each valid transaction included in a processed block.


.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
