*Needs Review*

Glossary
===========================

Terminology is important, so that all Fabric users and developers agree on what
we mean by each specific term. What is chaincode, for example. So we'll point you
there, whenever you want to reassure yourself. Of course, feel free to read the
entire thing in one sitting if you like, it's pretty enlightening!

.. _Anchor-Peer:

Anchor Peer
-----------

A peer node on a channel that all other peers can discover and communicate with.
Each Member_ on a channel has an anchor peer (or multiple anchor peers to prevent
single point of failure), allowing for peers belonging to different Members to
discover all existing peers on a channel.


.. _Block:

Block
-----

An ordered set of transactions that is cryptographically linked to the
preceding block(s) on a channel.

.. _Chain:

Chain
-----

The ledger's chain is a transaction log structured as hash-linked blocks of
transactions. Peers receive blocks of transactions from the ordering service, mark
the block's transactions as valid or invalid based on endorsement policies and
concurrency violations, and append the block to the hash chain on the peer's
file system.

.. _chaincode:

Chaincode
---------

Chaincode is software, running on a ledger, to encode assets and the transaction
instructions (business logic) for modifying the assets.

.. _Channel:

Channel
-------

A channel is a private blockchain overlay on a Fabric network, allowing for data
isolation and confidentiality. A channel-specific ledger is shared across the
peers in the channel, and transacting parties must be properly authenticated to
a channel in order to interact with it.  Channels are defined by a
Configuration-Block_.

.. _Commitment:

Commitment
----------

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

.. Consensus

Consensus
---------

A broader term overarching the entire transactional flow, which serves to generate
an agreement on the order and to confirm the correctness of the set of transactions
constituting a block.

.. _Current-State:

Current State
-------------

The current state of the ledger represents the latest values for all keys ever
included in its chain transaction log. Peers commit the latest values to ledger
current state for each valid transaction included in a processed block. Since
current state represents all latest key values known to the channel, it is
sometimes referred to as World State. Chaincode executes transaction proposals
against current state data.

.. _Dynamic-Membership:

Dynamic Membership
------------------

Fabric supports the addition/removal of members, peers, and ordering service
nodes, without compromising the operationality of the overall network. Dynamic
membership is critical when business relationships adjust and entities need to
be added/removed for various reasons.

.. _Endorsement:

Endorsement
-----------

Refers to the process where specific peer nodes execute a transaction and return
a ``YES/NO`` response to the client application that generated the transaction proposal.
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
misbehavior (deliberate or not) by the endorsing peers.  A distinct endorsement
policy for deploy transactions, which install new chaincode, is also required.

.. _Genesis-Block:

Genesis Block
-------------

The configuration block that initializes a blockchain network or channel, and
also serves as the first block on a chain.

.. _Gossip-Protocol:

Gossip Protocol
---------------

The gossip data dissemination protocol performs three functions:
1) manages peer discovery and channel membership;
2) disseminates ledger data across all peers on the channel;
3) syncs ledger state across all peers on the channel.
Refer to the :doc:`Gossip <gossip>` topic for more details.

.. _Initialize:

Initialize
----------

A method to initialize a chaincode application.

Install
-------

The process of placing a chaincode on a peer's file system.

Instantiate
-----------

The process of starting a chaincode container.

.. _Invoke:

Invoke
------

Used to call chaincode functions. Invocations are captured as transaction
proposals, which then pass through a modular flow of endorsement, ordering,
validation, committal. The structure of invoke is a function and an array of
arguments.

.. _Leading-Peer:

Leading Peer
------------

Each Member_ can own multiple peers on each channel that
it subscribes to. One of these peers is serves as the leading peer for the channel,
in order to communicate with the network ordering service on behalf of the
member. The ordering service "delivers" blocks to the leading peer(s) on a
channel, who then distribute them to other peers within the same member cluster.

.. _Ledger:

Ledger
------

A ledger is a channel's chain and current state data which is maintained by each
peer on the channel.

.. _Member:

Member
------

A legally separate entity that owns a unique root certificate for the network.
Network components such as peer nodes and application clients will be linked to a member.

.. _MSP:

Membership Service Provider
---------------------------

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

The ``fabric-ca`` component is an implementation of membership services to manage
identities. In particular, it handles the issuance and revocation of enrollment
certificates and transaction certificates.

An enrollment certificate is a long-term identity credential; a transaction
certificate is a short-term identity credential which is both anonymous and un-linkable.

.. _Ordering-Service:

Ordering Service
----------------

A defined collective of nodes that orders transactions into a block.  The ordering
service exists independent of the peer processes and orders transactions on a
first-come-first-serve basis for all channel's on the network.  The ordering service is
designed to support pluggable implementations beyond the out-of-the-box SOLO and Kafka varieties.
The ordering service is a common binding for the overall network; it contains the cryptographic
identity material tied to each Member_.

.. _Peer:

Peer
----

A network entity that maintains a ledger and runs chaincode containers in order to perform
read/write operations to the ledger.  Peers are owned and maintained by members.

.. _Policy:

Policy
------

There are policies for endorsement, validation, block committal, chaincode
management and network/channel management.

.. _Proposal:

Proposal
--------

A request for endorsement that is aimed at specific peers on a channel. Each
proposal is either an instantiate or an invoke (read/write) request.

.. _Query:

Query
-----

A query requests the value of a key(s) against the current state.

.. _SDK:

Software Development Kit (SDK)
------------------------------

The Hyperledger Fabric client SDK provides a structured environment of libraries
for developers to write and test chaincode applications. The SDK is fully
configurable and extensible through a standard interface. Components, including
cryptographic algorithms for signatures, logging frameworks and state stores,
are easily swapped in and out of the SDK. The SDK API uses protocol buffers over
gRPC for transaction processing, membership services, node traversal and event
handling applications to communicate across the fabric. The SDK comes in
multiple flavors - Node.js, Java. and Python.

.. _State-DB:

State Database
--------------

Current state data is stored in a state database for efficient reads and queries
from chaincode. These databases include levelDB and couchDB.

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

An invoke or instantiate operation.  Invokes are requests to read/write data from
the ledger.  Instantiate is a request to start a chaincode container on a peer.
