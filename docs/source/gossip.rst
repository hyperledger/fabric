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

Peers leverage gossip to establish a network of known available peers and
optionally broadcast ledger and channel data in a scalable fashion.
Each gossiped message is signed, thereby allowing Byzantine participants
sending faked messages to be easily identified and the distribution of the
message(s) to unwanted targets to be prevented.

The gossip-based data dissemination protocol performs several functions on
a Fabric network:

1. Manages peer discovery and channel membership, by continually
   identifying available member peers, and eventually detecting peers that have
   gone offline.
2. Disseminates private data between authorized peers on a channel.
3. Optionally disseminates pulled blocks across all peers on a channel, based on peer.deliveryclient.blockGossipEnabled setting
4. Optionally bring newly connected peers up to current block height by allowing peer-to-peer state
   transfer of blocks from other peers, based on peer.gossip.state.enabled setting.

Gossip-based broadcasting operates by peers receiving messages from
other peers on the channel, and then forwarding these messages to a number of
randomly selected peers on the channel, where this number is a configurable
constant. Peers can also exercise a pull mechanism rather than waiting for
delivery of a message. This cycle repeats, with the result of channel
membership, ledger and state information continually being kept current and in
sync. For new blocks, all **leader** peers on the channel pull
blocks from the ordering service and optionally initiate gossip dissemination to peers
in its own organization.

Leader election
---------------

The leader election mechanism is used to **elect** which peers per organization
will pull blocks from the ordering service and optionally initiate distribution of
newly arrived blocks across the peers of its own organization. Leveraging leader election
provides the system with the ability to efficiently utilize the bandwidth of the ordering
service. There are two possible modes of operation for a leader election module:

1. **Static** --- peer is statically configured to be a leader. Since v2.2 every peer is configured to be a static leader by default.
2. **Dynamic** --- peers execute a leader election procedure to select one peer in an organization to become leader.

Static leader election
~~~~~~~~~~~~~~~~~~~~~~

Static leader election allows you to manually define one or more peers within an organization as leader peers.
To enable static leader election mode, configure the following parameters within the section of ``core.yaml``:

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

2. Setting ``CORE_PEER_GOSSIP_USELEADERELECTION`` and ``CORE_PEER_GOSSIP_ORGLEADER``
   with ``true`` value is ambiguous and will lead to an error.
3. In static configuration organization admin is responsible to provide high availability
   of the leader node in case for failure or crashes.

Dynamic leader election
~~~~~~~~~~~~~~~~~~~~~~~

Dynamic leader election enables organization peers to **elect** one peer which will
connect to the ordering service and pull out new blocks. This leader is elected
for an organization's peers independently.

A dynamically elected leader sends **heartbeat** messages to the rest of the peers
as an evidence of liveness. If one or more peers don't receive **heartbeats** updates
during a set period of time, they will elect a new leader.

In the worst case scenario of a network partition, there will be more than one
active leader for organization to guarantee resiliency and availability to allow
an organization's peers to continue making progress. After the network partition
has been healed, one of the leaders will relinquish its leadership. In
a steady state with no network partitions, there will be
**only** one active leader connecting to the ordering service.

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

Alternatively these parameters could be configured and overridden with environment variables:

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

External and internal endpoints
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

In order for gossip to work effectively, peers need to be able to obtain the
endpoint information of peers in their own organization as well as from peers in
other organizations.

When a peer is bootstrapped it will use ``peer.gossip.bootstrap`` in its
``core.yaml`` to advertise itself and exchange membership information, building
a view of all available peers within its own organization.

The ``peer.gossip.bootstrap`` property in the ``core.yaml`` of the peer is
used to bootstrap gossip **within an organization**. If you are using gossip, you
will typically configure all the peers in your organization to point to an initial set of
bootstrap peers (you can specify a space-separated list of peers). The internal
endpoint is usually auto-computed by the peer itself or just passed explicitly
via ``core.peer.address`` in ``core.yaml``. If you need to overwrite this value,
you can export ``CORE_PEER_GOSSIP_ENDPOINT`` as an environment variable.

Bootstrap information is similarly required to establish communication **across
organizations**. The initial cross-organization bootstrap information is provided
via the "anchor peers" setting described above. If you want to make other peers
in your organization known to other organizations, you need to set the
``peer.gossip.externalendpoint`` in the ``core.yaml`` of your peer.
If this is not set, the endpoint information of the peer will not be broadcast
to peers in other organizations.

To set these properties, issue:

::

    export CORE_PEER_GOSSIP_BOOTSTRAP=<a list of peer endpoints within the peer's org>
    export CORE_PEER_GOSSIP_EXTERNALENDPOINT=<the peer endpoint, as known outside the org>

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

Because channels are segregated, peers on one channel cannot message or
share information on any other channel. Though any peer can belong
to multiple channels, partitioned messaging prevents blocks from being disseminated
to peers that are not in the channel by applying message routing policies based
on a peers' channel subscriptions.

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
