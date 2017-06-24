Channels
========

A Hyperledger Fabric **channel** is a private "subnet" of communication between
two or more specific network members, for the purpose of conducting private and
confidential transactions. A channel is defined by members (organizations),
anchor peers per member, the shared ledger, chaincode application(s) and the ordering service
node(s). Each transaction on the network is executed on a channel, where each
party must be authenticated and authorized to transact on that channel.
Each peer that joins a channel, has its own identity given by a membership services provider (MSP),
which authenticates each peer to its channel peers and services.

To create a new channel, the client SDK calls configuration system chaincode
and references properties such as **anchor peer**s, and members (organizations).
This request creates a **genesis block** for the channel ledger, which stores configuration
information about the channel policies, members and anchor peers. When adding a
new member to an existing channel, either this genesis block, or if applicable,
a more recent reconfiguration block, is shared with the new member.

.. note:: See the :doc:`configtx` section for more more details on the properties
          and proto structures of config transactions.

The election of a **leading peer** for each member on a channel determines which
peer communicates with the ordering service on behalf of the member. If no
leader is identified, an algorithm can be used to identify the leader. The consensus
service orders transactions and delivers them, in a block, to each leading peer,
which then distributes the block to its member peers, and across the channel,
using the **gossip** protocol.

Although any one anchor peer can belong to multiple channels, and therefore
maintain multiple ledgers, no ledger data can pass from one channel to another.
This separation of ledgers, by channel, is defined and implemented by
configuration chaincode, the identity membership service and the gossip data
dissemination protocol. The dissemination of data, which includes information on
transactions, ledger state and channel membership, is restricted to peers with
verifiable membership on the channel. This isolation of peers and ledger data,
by channel, allows network members that require private and confidential
transactions to coexist with business competitors and other restricted members,
on the same blockchain network.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/

