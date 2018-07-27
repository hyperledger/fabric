Gossip data dissemination protocol
==================================

Hyperledger Fabric optimizes blockchain network performance, security,
and scalability by dividing workload across transaction execution
(endorsing and committing) peers and transaction ordering nodes. This
decoupling of network operations requires a secure, reliable and
scalable data dissemination protocol to ensure data integrity and
consistency. To meet these requirements, Fabric implements a
**gossip data dissemination protocol**.

Gossip protocol
---------------

Peers leverage gossip to broadcast ledger and channel data in a scalable fashion.
Gossip messaging is continuous, and each peer on a channel is
constantly receiving current and consistent ledger data from multiple
peers. Each gossiped message is signed, thereby allowing Byzantine participants
sending faked messages to be easily identified and the distribution of the
message(s) to unwanted targets to be prevented. Peers affected by delays, network
partitions, or other causes resulting in missed blocks will eventually be
synced up to the current ledger state by contacting peers in possession of these
missing blocks.

The gossip-based data dissemination protocol performs three primary functions on
a Fabric network:

1. Manages peer discovery and channel membership, by continually
   identifying available member peers, and eventually detecting peers that have
   gone offline.
2. Disseminates ledger data across all peers on a channel. Any peer with data
   that is out of sync with the rest of the channel identifies the
   missing blocks and syncs itself by copying the correct data.
3. Bring newly connected peers up to speed by allowing peer-to-peer state
   transfer update of ledger data.

Gossip-based broadcasting operates by peers receiving messages from
other peers on the channel, and then forwarding these messages to a number of
randomly selected peers on the channel, where this number is a configurable
constant. Peers can also exercise a pull mechanism rather than waiting for
delivery of a message. This cycle repeats, with the result of channel
membership, ledger and state information continually being kept current and in
sync. For dissemination of new blocks, the **leader** peer on the channel pulls
the data from the ordering service and initiates gossip dissemination to peers
in its own organization.

Leader election
---------------

The leader election mechanism is used to **elect** one peer per each organization
which will maintain connection with ordering service and initiate distribution of
newly arrived blocks across peers of its own organization. Leveraging leader election
provides system with ability to efficiently utilize bandwidth of the ordering
service. There are two possible operation modes for leader election module:

1. **Static** -- system administrator manually configures one peer in the organization
   to be the leader, e.g. one to maintain open connection with the ordering service.
2. **Dynamic** -- peers execute a leader election procedure to select one peer in an
   organization to become leader, pull blocks from the ordering service, and disseminate
   blocks to the other peers in the organization.

Static leader election
~~~~~~~~~~~~~~~~~~~~~~

Using static leader election allows to manually define a set of leader peers within the organization, it's
possible to define a single node to be a leader or all available peers, it should be taken into account that -
making too many peers to connect to the ordering service might lead to inefficient bandwidth
utilization. To enable static leader election mode, configure the following parameters
within the section of ``core.yaml``:

::

    peer:
        # Gossip related configuration
        gossip:
            useLeaderElection: false
            orgLeader: true

Alternatively these parameters could be configured and overridden with environmental variables:

::

    export CORE_PEER_GOSSIP_USELEADERELECTION=false
    export CORE_PEER_GOSSIP_ORGLEADER=true


.. note:: The following configuration will keep peer in **stand-by** mode, i.e.
          peer will not try to become a leader:

::

    export CORE_PEER_GOSSIP_USELEADERELECTION=false
    export CORE_PEER_GOSSIP_ORGLEADER=false

2. Setting ``CORE_PEER_GOSSIP_USELEADERELECTION`` and ``CORE_PEER_GOSSIP_USELEADERELECTION``
   with ``true`` value is ambiguous and will lead to an error.
3. In static configuration organization admin is responsible to provide high availability
   of the leader node in case for failure or crashes.


Dynamic leader election
~~~~~~~~~~~~~~~~~~~~~~~

Dynamic leader election enables organization peers to **elect** one peer which will
connect to the ordering service and pull out new blocks. Leader is elected for set
of peers for each organization independently.

Elected leader is responsible to send the **heartbeat** messages to the rest of the peers
as an evidence of liveness. If one or more peers won't get **heartbeats** updates during
period of time, they will initiate a new round of leader election procedure, eventually
selecting a new leader. In case of a network partition in the worst case
there will be more than one active leader for organization thus to guarantee resiliency
and availability allowing the organization's peers to continue making progress. After
the network partition is healed one of the leaders will relinquish its leadership, therefore in
steady state and in no presence of network partitions for each organization there will be **only**
one active leader connecting to the ordering service.

Following configuration controls frequency of the leader **heartbeat** messages:

::

    peer:
        # Gossip related configuration
        gossip:
            election:
                leaderAliveThreshold: 10s

In order to enable dynamic leader election, the following parameters need to be configured
within ``core.yaml``:

::

    peer:
        # Gossip related configuration
        gossip:
            useLeaderElection: true
            orgLeader: false

Alternatively these parameters could be configured and overridden with environmental variables:

::

    export CORE_PEER_GOSSIP_USELEADERELECTION=true
    export CORE_PEER_GOSSIP_ORGLEADER=false

Anchor peers
------------

Anchor peers are used by gossip to make sure peers in different organizations
know about each other.

When a configuration block that contains an update to the anchor peers is committed,
peers reach out to the anchor peers and learn from them about all of the peers known
to the anchor peer(s). Once at least one peer from each organization has contacted an
anchor peer, the anchor peer learns about every peer in the channel. Since gossip
communication is constant, and because peers always ask to be told about the existence
of any peer they don't know about, a common view of membership can be established for
a channel.

For example, let's assume we have three organizations---`A`, `B`, `C`--- in the channel
and a single anchor peer---`peer0.orgC`--- defined for organization `C`. When `peer1.orgA`
(from organization `A`) contacts `peer0.orgC`, it will tell it about `peer0.orgA`. And
when at a later time `peer1.orgB` contacts `peer0.orgC`, the latter would tell the
former about `peer0.orgA`. From that point forward, organizations `A` and `B` would
start exchanging membership information directly without any assistance from
`peer0.orgC`.

As communication across organizations depends on gossip in order to work, there must
be at least one anchor peer defined in the channel configuration. It is strongly
recommended that every organization provides its own set of anchor peers for high
availability and redundancy. Note that the anchor peer does not need to be the
same peer as the leader peer.


Gossip messaging
----------------

Online peers indicate their availability by continually broadcasting "alive"
messages, with each containing the **public key infrastructure (PKI)** ID and the
signature of the sender over the message. Peers maintain channel membership by collecting
these alive messages; if no peer receives an alive message from a specific peer,
this "dead" peer is eventually purged from channel membership. Because "alive"
messages are cryptographically signed, malicious peers can never impersonate
other peers, as they lack a signing key authorized by a root certificate
authority (CA).

In addition to the automatic forwarding of received messages, a state
reconciliation process synchronizes **world state** across peers on each
channel. Each peer continually pulls blocks from other peers on the channel,
in order to repair its own state if discrepancies are identified. Because fixed
connectivity is not required to maintain gossip-based data dissemination, the
process reliably provides data consistency and integrity to the shared ledger,
including tolerance for node crashes.

Because channels are segregated, peers on one channel cannot message or
share information on any other channel. Though any peer can belong
to multiple channels, partitioned messaging prevents blocks from being disseminated
to peers that are not in the channel by applying message routing policies based
on peers' channel subscriptions.

.. note:: 1. Security of point-to-point messages are handled by the peer TLS layer, and do
          not require signatures. Peers are authenticated by their certificates,
          which are assigned by a CA. Although TLS certs are also used, it is
          the peer certificates that are authenticated in the gossip layer. Ledger blocks
          are signed by the ordering service, and then delivered to the leader peers on a channel.

          2. Authentication is governed by the membership service provider for the
          peer. When the peer connects to the channel for the first time, the
          TLS session binds with the membership identity. This essentially
          authenticates each peer to the connecting peer, with respect to
          membership in the network and channel.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
