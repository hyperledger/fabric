# The Ordering Service

**Audience:** Architects, ordering service admins, channel creators

This topic serves as a conceptual introduction to the concept of ordering, how
orderers interact with peers, the role they play in a transaction flow, and an
overview of the currently available implementations of the ordering service,
with a particular focus on the recommended **Raft** ordering service implementation.

## What is ordering?

Many distributed blockchains, such as Ethereum and Bitcoin, are not permissioned,
which means that any node can participate in the consensus process, wherein
transactions are ordered and bundled into blocks. Because of this fact, these
systems rely on **probabilistic** consensus algorithms which eventually
guarantee ledger consistency to a high degree of probability, but which are
still vulnerable to divergent ledgers (also known as a ledger "fork"), where
different participants in the network have a different view of the accepted
order of transactions.

Hyperledger Fabric works differently. It features a node called an
**orderer** (it's also known as an "ordering node") that does this transaction
ordering, which along with other orderer nodes forms an **ordering service**.
Because Fabric's design relies on **deterministic** consensus algorithms, any block
validated by the peer is guaranteed to be final and correct. Ledgers cannot fork
the way they do in many other distributed and permissionless blockchain networks.

In addition to promoting finality, separating the endorsement of chaincode
execution (which happens at the peers) from ordering gives Fabric advantages
in performance and scalability, eliminating bottlenecks which can occur when
execution and ordering are performed by the same nodes.

## Orderer nodes and channel configuration

Orderers also enforce basic access control for channels, restricting who can
read and write data to them, and who can configure them. Remember that who
is authorized to modify a configuration element in a channel is subject to the
policies that the relevant administrators set when they created the channel.
Configuration transactions are processed by the orderer,
as it needs to know the current set of policies to execute its basic
form of access control. In this case, the orderer processes the
configuration update to make sure that the requestor has the proper
administrative rights. If so, the orderer validates the update request against
the existing configuration, generates a new configuration transaction,
and packages it into a block that is relayed to all peers on the channel. The
peers then process the configuration transactions in order to verify that the
modifications approved by the orderer do indeed satisfy the policies defined in
the channel.

## Orderer nodes and identity

Everything that interacts with a blockchain network, including peers,
applications, admins, and orderers, acquires their organizational identity from
their digital certificate and their Membership Service Provider (MSP) definition.

For more information about identities and MSPs, check out our documentation on
[Identity](../identity/identity.html) and [Membership](../membership/membership.html).

Just like peers, ordering nodes belong to an organization. And similar to peers,
a separate Certificate Authority (CA) should be used for each organization.
Whether this CA will function as the root CA, or whether you choose to deploy
a root CA and then intermediate CAs associated with that root CA, is up to you.

## Orderers and the transaction flow

### Phase one: Transaction Proposal and Endorsement

We've seen from our topic on [Peers](../peers/peers.html) that they form the basis
for a blockchain network, hosting ledgers, which can be queried and updated by
applications through smart contracts.

Specifically, applications that want to update the ledger are involved in a
process with three phases that ensures all of the peers in a blockchain network
keep their ledgers consistent with each other.

In the first phase, a client application sends a transaction proposal to the Fabric
Gateway service, via a trusted peer. This peer executes the proposed transaction or
forwards it to another peer in its organization for execution.

The gateway also forwards the transaction to peers in the organizations required by the endorsement policy. These endorsing peers run the transaction and return the
transaction response to the gateway service. They do not apply the proposed update to
their copy of the ledger at this time. The endorsed transaction proposals will ultimately
be ordered into blocks in phase two, and then distributed to all peers for final validation
and commitment to the ledger in phase three.

Note: Fabric v2.3 SDKs embed the logic of the v2.4 Fabric Gateway service in the client application --- refer to the [v2.3 Applications and Peers](https://hyperledger-fabric.readthedocs.io/en/release-2.3/peers/peers.html#applications-and-peers) topic for details.

For an in-depth look at phase one, refer back to the [Peers](../peers/peers.html#applications-and-peers) topic.

### Phase two: Transaction Submission and Ordering

With successful completion of the first transaction phase (proposal), the client
application has received an endorsed transaction proposal response from the
Fabric Gateway service for signing. For an endorsed transaction, the gateway service
forwards the transaction to the ordering service, which orders it with
other endorsed transactions, and packages them all into a block.

The ordering service creates these blocks of transactions, which will ultimately
be distributed to all peers on the channel for validation and commitment to
the ledger in phase three. The blocks themselves are also ordered and are the
basic components of a blockchain ledger.

Ordering service nodes receive transactions from many different application
clients (via the gateway) concurrently. These ordering service nodes collectively
form the ordering service, which may be shared by multiple channels.

The number of transactions in a block depends on channel configuration
parameters related to the desired size and maximum elapsed duration for a
block (`BatchSize` and `BatchTimeout` parameters, to be exact). The blocks are
then saved to the orderer's ledger and distributed to all peers on the channel.
If a peer happens to be down at this time, or joins
the channel later, it will receive the blocks by gossiping with another peer.
We'll see how this block is processed by peers in the third phase.

It's worth noting that the sequencing of transactions in a block is not
necessarily the same as the order received by the ordering service, since there
can be multiple ordering service nodes that receive transactions at approximately
the same time.  What's important is that the ordering service puts the transactions
into a strict order, and peers will use this order when validating and committing
transactions.

This strict ordering of transactions within blocks makes Hyperledger Fabric a
little different from other blockchains where the same transaction can be
packaged into multiple different blocks that compete to form a chain.
In Hyperledger Fabric, the blocks generated by the ordering service are
**final**. Once a transaction has been written to a block, its position in the
ledger is immutably assured. As we said earlier, Hyperledger Fabric's finality
means that there are no **ledger forks** --- validated and committed transactions
will never be reverted or dropped.

We can also see that, whereas peers execute smart contracts (chaincode) and process transactions,
orderers most definitely do not. Every authorized transaction that arrives at an
orderer is then mechanically packaged into a block --- the orderer makes no judgement
as to the content of a transaction (except for channel configuration transactions,
as mentioned earlier).

At the end of phase two, we see that orderers have been responsible for the simple
but vital processes of collecting proposed transaction updates, ordering them,
and packaging them into blocks, ready for distribution to the channel peers.

### Phase three: Transaction Validation and Commitment

The third phase of the transaction workflow involves the distribution of
ordered and packaged blocks from the ordering service to the channel peers
for validation and commitment to the ledger.

Phase three begins with the ordering service distributing blocks to all channel
peers. It's worth noting that not every peer needs to be connected to an orderer ---
peers can cascade blocks to other peers using the [**gossip**](../gossip.html)
protocol --- although receiving blocks directly from the ordering service is
recommended.

Each peer will validate distributed blocks independently, ensuring that ledgers
remain consistent. Specifically, each peer in the channel will validate each
transaction in the block to ensure it has been endorsed
by the required organizations, that its endorsements match, and that
it hasn't become invalidated by other recently committed transactions. Invalidated
transactions are still retained in the immutable block created by the orderer,
but they are marked as invalid by the peer and do not update the ledger's state.

![Orderer2](./orderer.diagram.2.png)

*The second role of an ordering node is to distribute blocks to peers. In this
example, orderer O1 distributes block B2 to peer P1 and peer P2. Peer P1
processes block B2, resulting in a new block being added to ledger L1 on P1. In
parallel, peer P2 processes block B2, resulting in a new block being added to
ledger L1 on P2. Once this process is complete, the ledger L1 has been
consistently updated on peers P1 and P2, and each may inform connected
applications that the transaction has been processed.*

In summary, phase three sees the blocks of transactions created by the ordering
service applied consistently to the ledger by the peers. The strict
ordering of transactions into blocks allows each peer to validate that transaction
updates are consistently applied across the channel.

For a deeper look at phase 3, refer back to the [Peers](../peers/peers.html#phase-3-validation-and-commit) topic.

## Ordering service implementations

While every ordering service currently available handles transactions and
configuration updates the same way, there are nevertheless several different
implementations for achieving consensus on the strict ordering of transactions
between ordering service nodes.

For information about how to stand up an ordering node (regardless of the
implementation the node will be used in), check out [our documentation on deploying a production ordering service](../deployorderer/ordererplan.html).

* **Raft**

  New as of v1.4.1, Raft is a crash fault tolerant (CFT) ordering service
  based on an implementation of [Raft protocol](https://raft.github.io/raft.pdf)
  in [`etcd`](https://coreos.com/etcd/). Raft follows a "leader and
  follower" model, where a leader node is elected (per channel) and its decisions
  are replicated by the followers. Raft ordering services should be easier to set
  up and manage than Kafka-based ordering services, and their design allows
  different organizations to contribute nodes to a distributed ordering service.

* **BFT** (New as of v3.0)

  A Byzantine Fault Tolerant (BFT) ordering service, as its name implies,
  can withstand not only crash failures, but also a subset of nodes behaving maliciously.
  It is now possible to run a BFT ordering service
  with the [SmartBFT](https://arxiv.org/abs/2107.06922) [library](https://github.com/SmartBFT-Go/consensus)
  as its underlying consensus protocol. Consider using the BFT orderer if true decentralization is required, where
  up to and not including a third of the parties running the orderers may not be trusted due to malicious intent or being compromised.

* **Kafka**
  Kafka was deprecated in v2.x and is no longer supported in v3.x

* **Solo** (deprecated in v2.x)

  The Solo implementation of the ordering service is intended for test only and
  consists only of a single ordering node. It has been deprecated and may be
  removed entirely in a future release. Existing users of Solo should move to
  a single node Raft network for equivalent function.

## Raft

For information on how to customize the `orderer.yaml` file that determines the configuration of an ordering node, check out the [Checklist for a production ordering node](../deployorderer/ordererchecklist.html).

The go-to ordering service choice for production networks, the Fabric
implementation of the established Raft protocol uses a "leader and follower"
model, in which a leader is dynamically elected among the ordering
nodes in a channel (this collection of nodes is known as the "consenter set"),
and that leader replicates messages to the follower nodes. Because the system
can sustain the loss of nodes, including leader nodes, as long as there is a
majority of ordering nodes (what's known as a "quorum") remaining, Raft is said
to be "crash fault tolerant" (CFT). In other words, if there are three nodes in a
channel, it can withstand the loss of one node (leaving two remaining). If you
have five nodes in a channel, you can lose two nodes (leaving three
remaining nodes). This feature of a Raft ordering service is a factor in the
establishment of a high availability strategy for your ordering service. Additionally,
in a production environment, you would want to spread these nodes across data
centers and even locations. For example, by putting one node in three different
data centers. That way, if a data center or entire location becomes unavailable,
the nodes in the other data centers continue to operate.

* The Fabric Raft implementation has been developed and will be supported within the Fabric
developer community and its support apparatus.

* Raft allows the users to specify which ordering nodes will
be deployed to which channel. In this way, peer organizations can make sure
that, if they also own an orderer, this node will be made a part of an ordering
service of that channel, rather than trusting and depending on a central admin
to manage the nodes.

* Raft is the first step toward Fabric's development of a byzantine fault tolerant
(BFT) ordering service. As we'll see, some decisions in the development of
Raft were driven by this. If you are interested in BFT, learning how to use
Raft should ease the transition.

Note: Similar to Solo, a Raft ordering service can lose transactions
after acknowledgement of receipt has been sent to a client. For example, if the
leader crashes at approximately the same time as a follower provides
acknowledgement of receipt. Therefore, application clients should listen on peers
for transaction commit events regardless (to check for transaction validity), but
extra care should be taken to ensure that the client also gracefully tolerates a
timeout in which the transaction does not get committed in a configured timeframe.
Depending on the application, it may be desirable to resubmit the transaction or
collect a new set of endorsements upon such a timeout.

### Raft concepts

Raft offers many features in a simple and easy-to-use package
and introduces a number of new concepts, or twists on existing concepts, to Fabric.

**Log entry**. The primary unit of work in a Raft ordering service is a "log
entry", with the full sequence of such entries known as the "log". We consider
the log consistent if a majority (a quorum, in other words) of members agree on
the entries and their order, making the logs on the various orderers replicated.

**Consenter set**. The ordering nodes actively participating in the consensus
mechanism for a given channel and receiving replicated logs for the channel.

**Finite-State Machine (FSM)**. Every ordering node in Raft has an FSM and
collectively they're used to ensure that the sequence of logs in the various
ordering nodes is deterministic (written in the same sequence).

**Quorum**. Describes the minimum number of consenters that need to affirm a
proposal so that transactions can be ordered. For every consenter set, this is a
**majority** of nodes. In a cluster with five nodes, three must be available for
there to be a quorum. If a quorum of nodes is unavailable for any reason, the
ordering service cluster becomes unavailable for both read and write operations
on the channel, and no new logs can be committed.

**Leader**. This is not a new concept, but it's critical to understand that at any given time,
a channel's consenter set elects a single node to be the leader (we'll describe how
this happens in Raft later). The leader is responsible for ingesting new log entries,
replicating them to follower ordering nodes, and managing when an entry is considered
committed. This is not a special **type** of orderer. It is only a role that
an orderer may have at certain times, and then not others, as circumstances
determine.

**Follower**. Again, not a new concept, but what's critical to understand about
followers is that the followers receive the logs from the leader and
replicate them deterministically, ensuring that logs remain consistent. As
we'll see in our section on leader election, the followers also receive
"heartbeat" messages from the leader. In the event that the leader stops
sending those message for a configurable amount of time, the followers will
initiate a leader election and one of them will be elected the new leader.

### Raft in a transaction flow

Every channel runs on a **separate** instance of the Raft protocol, which allows each instance to elect a different leader. This configuration also allows further decentralization of the service in use cases where clusters are made up of ordering nodes controlled by different organizations. Ordering nodes can be added or removed from a channel as needed as long as only a single node is added or removed at a time. While this configuration creates more overhead in the form of redundant heartbeat messages and goroutines, it lays necessary groundwork for BFT.

In Raft, transactions (in the form of proposals or configuration updates) are
automatically routed by the ordering node that receives the transaction to the
current leader of that channel. This means that peers and applications do not
need to know who the leader node is at any particular time. Only the ordering
nodes need to know.

When the orderer validation checks have been completed, the transactions are
ordered, packaged into blocks, consented on, and distributed, as described in
phase two of our transaction flow.

### Architectural notes

#### How leader election works in Raft

Although the process of electing a leader happens within the orderer's internal
processes, it's worth noting how the process works.

Raft nodes are always in one of three states: follower, candidate, or leader.
All nodes initially start out as a **follower**. In this state, they can accept
log entries from a leader (if one has been elected), or cast votes for leader.
If no log entries or heartbeats are received for a set amount of time (for
example, five seconds), nodes self-promote to the **candidate** state. In the
candidate state, nodes request votes from other nodes. If a candidate receives a
quorum of votes, then it is promoted to a **leader**. The leader must accept new
log entries and replicate them to the followers.

For a visual representation of how the leader election process works, check out
[The Secret Lives of Data](http://thesecretlivesofdata.com/raft/).

#### Snapshots

If an ordering node goes down, how does it get the logs it missed when it is
restarted?

While it's possible to keep all logs indefinitely, in order to save disk space,
Raft uses a process called "snapshotting", in which users can define how many
bytes of data will be kept in the log. This amount of data will conform to a
certain number of blocks (which depends on the amount of data in the blocks.
Note that only full blocks are stored in a snapshot).

For example, let's say lagging replica `R1` was just reconnected to the network.
Its latest block is `100`. Leader `L` is at block `196`, and is configured to
snapshot at amount of data that in this case represents 20 blocks. `R1` would
therefore receive block `180` from `L` and then make a `Deliver` request for
blocks `101` to `180`. Blocks `180` to `196` would then be replicated to `R1`
through the normal Raft protocol.


## BFT

For information on how to deploy and manage the BFT orderer, be sure to check out the [deployment guide](../bft_configuration.md).

The protocol used by Fabric's BFT orderer implementation is the [SmartBFT](https://arxiv.org/abs/2107.0692) protocol
heavily inspired by the [BFT-SMART](https://www.di.fc.ul.pt/~bessani/publications/dsn14-bftsmart.pdf) protocol,
which itself can be thought of as a non-pipelined(*) version of the seminal [PBFT](https://pmg.csail.mit.edu/papers/osdi99.pdf) protocol.
As in Raft, the protocol designates a single leader which batches transactions into a block and sends them to the rest of the nodes, termed followers.
However, unlike Raft, the leader is not dynamically selected but is rotated in a round-robin fashion every time the previous leader is suspected of being faulty
by the follower nodes.

Further differentiating itself from Raft, where more than half of the nodes are required for the ordering service to be functional,
the BFT protocol withstands failures of up to (and not including) a third of the nodes.
If a third or more of the nodes crash or are unreachable, no blocks can be agreed upon.

The advantage of the BFT orderer over the Raft orderer is that it can withstand some of the nodes being compromised.
Indeed, if up to (but not including) a third of the orderer nodes are controlled by a malicous party,
the system can still accept new transactions, order them, and most importantly ensure the same blocks are committed
by the rest of the ordering nodes. This is in contrast to Raft, which is not suitable to be deployed in such a harsh adversary model.

Operating the BFT orderer is identical to how the Raft orderer is operated: New nodes can be added and removed from
the channel dynamically and while the system is running, and it is described in the [reconfiguration guide](../create_channel/add_orderer.md).

Similarly to Raft, the BFT leader sends periodical heartbeats to each follower, and if the latter does not hear
from the leader within a period of time, it starts lobbying other followers to change the leader.

A major difference from Raft, is that when a client submits a transaction to a BFT ordering service, it should send
the transaction to all nodes instead of to a single node. Sending the transaction to all nodes will not only
make sure it has reached the leader node, but will also ensure that even if the leader node is malicious
and is ignoring the transaction of the client, it will eventually have no choice but to include the transaction
in some future block, or face being overthrown by the follower nodes which have received the transaction
from the client and lost their patience waiting for it being included in a block sent by the leader.

While sending the transaction to all nodes may seem as a disadvantage, the BFT transaction semantics actually
harbor an implicit advantage over the Raft transactional semantics: In Raft, sending a transaction to an orderer
does not guarantee its inclusion in a block, and neither does sending it to all nodes, as the leader may crash and
then the transaction may be lost. In BFT, however, even if the leader crashes, the transaction will still remain
in the memory of the follower nodes, and will eventually be either sent to the leader and then included in a block,
or the leader will be forced to change to a new leader that will eventually include the transaction.

Applications that submit their transactions through the [gateway service](../gateway.md) do not need to change
anything, as the gateway service knows whether it should submit to all ordering nodes or just to one
based on the configuration of the channel, and acts accordingly.



(*) A consensus protocol without a pipeline agrees on a block only after the previous block has been agreed upon.



<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/) -->
