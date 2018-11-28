Service Discovery
=================

Why do we need service discovery?
---------------------------------

In order to execute chaincode on peers, submit transactions to orderers, and to
be updated about the status of transactions, applications connect to an API
exposed by an SDK.

However, the SDK needs a lot of information in order to allow applications to
connect to the relevant network nodes. In addition to the CA and TLS certificates
of the orderers and peers on the channel -- as well as their IP addresses and port
numbers -- it must know the relevant endorsement policies as well as which peers
have the chaincode installed (so the application knows which peers to send chaincode
proposals to).

Prior to v1.2, this information was statically encoded. However, this implementation
is not dynamically reactive to network changes (such as the addition of peers who have
installed the relevant chaincode, or peers that are temporarily offline). Static
configurations also do not allow applications to react to changes of the
endorsement policy itself (as might happen when a new organization joins a channel).

In addition, the client application has no way of knowing which peers have updated
ledgers and which do not. As a result, the application might submit proposals to
peers whose ledger data is not in sync with the rest of the network, resulting
in transaction being invalidated upon commit and wasting resources as a consequence.

The **discovery service** improves this process by having the peers compute
the needed information dynamically and present it to the SDK in a consumable
manner.

How service discovery works in Fabric
-------------------------------------

The application is bootstrapped knowing about a group of peers which are
trusted by the application developer/administrator to provide authentic responses
to discovery queries. A good candidate peer to be used by the client application
is one that is in the same organization. Note that in order for peers to be known
to the discovery service, they must have an ``EXTERNAL_ENDPOINT`` defined. To see
how to do this, check out our :doc:`discovery-cli` documentation.

The application issues a configuration query to the discovery service and obtains
all the static information it would have otherwise needed to communicate with the
rest of the nodes of the network. This information can be refreshed at any point
by sending a subsequent query to the discovery service of a peer.

The service runs on peers -- not on the application -- and uses the network metadata
information maintained by the gossip communication layer to find out which peers
are online. It also fetches information, such as any relevant endorsement policies,
from the peer's state database.

With service discovery, applications no longer need to specify which peers they
need endorsements from. The SDK can simply send a query to the discovery service
asking which peers are needed given a channel and a chaincode ID. The discovery
service will then compute a descriptor comprised of two objects:

1. **Layouts**: a list of groups of peers and a corresponding amount of peers from
   each group which should be selected.
2. **Group to peer mapping**: from the groups in the layouts to the peers of the
   channel. In practice, each group would most likely be peers that represent
   individual organizations, but because the service API is generic and ignorant of
   organizations this is just a "group".

The following is an example of a descriptor from the evaluation of a policy of
``AND(Org1, Org2)`` where there are two peers in each of the organizations.

.. code-block:: JSON

   Layouts: [
        QuantitiesByGroup: {
          “Org1”: 1,
          “Org2”: 1,
        }
   ],
   EndorsersByGroups: {
     “Org1”: [peer0.org1, peer1.org1],
     “Org2”: [peer0.org2, peer1.org2]
   }

In other words, the endorsement policy requires a signature from one peer in Org1
and one peer in Org2. And it provides the names of available peers in those orgs who
can endorse (``peer0`` and ``peer1`` in both Org1 and in Org2).

The SDK then selects a random layout from the list. In the example above, the
endorsement policy is Org1 ``AND`` Org2. If instead it was an ``OR`` policy, the SDK
would randomly select either Org1 or Org2, since a signature from a peer from either
Org would satisfy the policy.

After the SDK has selected a layout, it selects from the peers in the layout based on a
criteria specified on the client side (the SDK can do this because it has access to
metadata like ledger height). For example, it can prefer peers with higher ledger heights
over others -- or to exclude peers that the application has discovered to be offline
-- according to the number of peers from each group in the layout. If no single
peer is preferable based on the criteria, the SDK will randomly select from the peers
that best meet the criteria.

Capabilities of the discovery service
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The discovery service can respond to the following queries:

* **Configuration query**: Returns the ``MSPConfig`` of all organizations in the channel
  along with the orderer endpoints of the channel.
* **Peer membership query**: Returns the peers that have joined the channel.
* **Endorsement query**: Returns an endorsement descriptor for given chaincode(s) in
  a channel.
* **Local peer membership query**: Returns the local membership information of the
  peer that responds to the query. By default the client needs to be an administrator
  for the peer to respond to this query.

Special requirements
~~~~~~~~~~~~~~~~~~~~~~
When the peer is running with TLS enabled the client must provide a TLS certificate when connecting
to the peer. If the peer isn't configured to verify client certificates (clientAuthRequired is false), this TLS certificate
can be self-signed.

.. Licensed under Creative Commons Attribution 4.0 International License
   https://creativecommons.org/licenses/by/4.0/
