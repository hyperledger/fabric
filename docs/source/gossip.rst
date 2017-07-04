Gossip data dissemination protocol
==================================

Hyperledger Fabric optimizes blockchain network performance, security
and scalability by dividing workload across transaction execution
(endorsing and committing) peers and transaction ordering nodes. This
decoupling of network operations requires a secure, reliable and
scalable data dissemination protocol to ensure data integrity and
consistency. To meet these requirements, Hyperledger Fabric implements a
**gossip data dissemination protocol**.

Gossip protocol
---------------

Peers leverage gossip to broadcast ledger and channel data in a scalable fashion.
Gossip messaging is continuous, and each peer on a channel is
constantly receiving current and consistent ledger data, from multiple
peers. Each gossiped message is signed, thereby allowing Byzantine participants
sending faked messages to be easily identified and the distribution of the
message(s) to unwanted targets to be prevented. Peers affected by delays, network
partitions or other causations resulting in missed blocks, will eventually be
synced up to the current ledger state by contacting peers in possession of these
missing blocks.

The gossip-based data dissemination protocol performs three primary functions on
a Hyperledger Fabric network:

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
randomly-selected peers on the channel, where this number is a configurable
constant. Peers can also exercise a pull mechanism, rather than waiting for
delivery of a message.  This cycle repeats, with the result of channel
membership, ledger and state information continually being kept current and in
sync. For dissemination of new blocks, the **leader** peer on the channel pulls
the data from the ordering service and initiates gossip dissemination to peers.

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

| **Notes:**
| 1. Security of point-to-point messages are handled by the peer TLS layer, and do
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

